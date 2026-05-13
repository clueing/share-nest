package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"file-service/internal/config"
	"file-service/internal/model"
	"file-service/internal/preview"
	"file-service/internal/repo"
	"file-service/internal/security"
	"file-service/internal/storage"
	"file-service/internal/ui"
	qrcode "github.com/skip2/go-qrcode"
)

const adminCookieName = "fs_admin_session"
const adminFlashCookieName = "fs_admin_flash"

type Server struct {
	cfg       config.Config
	repo      *repo.SQLiteRepo
	storage   *storage.Local
	templates *template.Template
	mux       *http.ServeMux
	reqSeq    atomic.Uint64
}

type dashboardData struct {
	SiteName      string
	BaseURL       string
	Items         []model.ItemSummary
	AccessLogs    []model.AccessLog
	Message       string
	Flash         flashData
	EnvPath       string
	EnvExample    string
	MaxUploadSize int64
	CurrentPage   int
	TotalPages    int
	TotalCount    int
	PageSize      int
	PrevPage      int
	NextPage      int
	PageNumbers   []int
}

type sharePageData struct {
	SiteName          string
	Item              model.SharedItem
	ShareURL          string
	Locked            bool
	Error             string
	Expired           bool
	NoDownloads       bool
	CanCopyText       bool
	CopyTextSource    string
	DownloadsLeft     int
	PreviewMode       preview.Mode
	TextHTML          template.HTML
	MarkdownHTML      template.HTML
	CodeThemeCSS      template.CSS
	ArchivePreview    *preview.ArchiveSummary
	IsMarkdown        bool
	CodeLanguageLabel string
	Truncated         bool
	PreviewLimit      int64
	RawURL            string
	DownloadURL       string
}

type flashData struct {
	Message       string `json:"message"`
	ShareURL      string `json:"share_url"`
	DirectURL     string `json:"direct_url"`
	PasswordURL   string `json:"password_url"`
	SharePassword string `json:"share_password"`
	AutoCopy      string `json:"auto_copy"`
}

type actionResponse struct {
	OK       bool       `json:"ok"`
	Message  string     `json:"message,omitempty"`
	Redirect string     `json:"redirect,omitempty"`
	Flash    *flashData `json:"flash,omitempty"`
}

type preparedContent struct {
	file    *os.File
	modTime time.Time
}

func New(cfg config.Config, repo *repo.SQLiteRepo, fileStorage *storage.Local) (*Server, error) {
	tmpl, err := template.New("pages").Funcs(template.FuncMap{
		"humanSize": humanSize,
		"formatTime": func(value any) string {
			switch v := value.(type) {
			case time.Time:
				return v.Local().Format("2006-01-02 15:04")
			case int64:
				return formatUnix(v)
			default:
				if t, ok := value.(interface {
					Unix() int64
				}); ok {
					return formatUnix(t.Unix())
				}
			}
			return ""
		},
	}).ParseFS(ui.Files, "templates/*.html")
	if err != nil {
		return nil, err
	}

	staticFS, err := fs.Sub(ui.Files, "static")
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:       cfg,
		repo:      repo,
		storage:   fileStorage,
		templates: tmpl,
		mux:       http.NewServeMux(),
	}

	s.routes(http.FS(staticFS))
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.withMiddlewares(s.mux)
}

func (s *Server) routes(staticFS http.FileSystem) {
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	s.mux.Handle("GET /favicon.ico", http.FileServer(staticFS))
	s.mux.HandleFunc("GET /qr.png", s.handleQRCodeImage)

	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.handleLogout)
	s.mux.HandleFunc("GET /admin", s.requireAdmin(s.handleDashboard))
	s.mux.HandleFunc("POST /admin/upload", s.requireAdmin(s.handleUpload))
	s.mux.HandleFunc("POST /admin/text", s.requireAdmin(s.handleCreateText))
	s.mux.HandleFunc("POST /admin/items/{id}/delete", s.requireAdmin(s.handleDeleteItem))
	s.mux.HandleFunc("POST /admin/items/batch-delete", s.requireAdmin(s.handleBatchDelete))
	s.mux.HandleFunc("GET /s/{code}", s.handleSharePage)
	s.mux.HandleFunc("POST /s/{code}/verify", s.handleShareVerify)
	s.mux.HandleFunc("GET /s/{code}/raw", s.handleShareRaw)
	s.mux.HandleFunc("GET /s/{code}/download", s.handleShareDownload)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if s.isAdminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.isAdminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	s.render(w, "login", map[string]any{
		"Error":    r.URL.Query().Get("error"),
		"SiteName": s.cfg.SiteName,
	}, http.StatusOK)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("请求格式错误"), http.StatusSeeOther)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if !security.EqualString(username, s.cfg.AdminUser) || !security.EqualString(password, s.cfg.AdminPass) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("用户名或密码错误"), http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    s.adminCookieValue(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isHTTPSRequest(r),
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isHTTPSRequest(r),
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	currentPage := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := s.cfg.PageSize
	offset := (currentPage - 1) * pageSize

	items, totalCount, err := s.repo.ListItemsPage(r.Context(), offset, pageSize)
	if err != nil {
		http.Error(w, "加载资源列表失败", http.StatusInternalServerError)
		return
	}

	totalPages := maxInt(1, (totalCount+pageSize-1)/pageSize)
	if totalCount == 0 {
		totalPages = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
		offset = (currentPage - 1) * pageSize
		items, totalCount, err = s.repo.ListItemsPage(r.Context(), offset, pageSize)
		if err != nil {
			http.Error(w, "加载资源列表失败", http.StatusInternalServerError)
			return
		}
	}

	flash := s.readFlash(w, r)
	logs, err := s.repo.ListRecentAccessLogs(r.Context(), 20)
	if err != nil {
		http.Error(w, "加载访问日志失败", http.StatusInternalServerError)
		return
	}
	data := dashboardData{
		SiteName:      s.cfg.SiteName,
		BaseURL:       s.baseURL(r),
		Items:         items,
		AccessLogs:    logs,
		Message:       flash.Message,
		Flash:         flash,
		EnvPath:       ".env",
		EnvExample:    s.envExample(),
		MaxUploadSize: s.cfg.MaxUploadSize,
		CurrentPage:   currentPage,
		TotalPages:    totalPages,
		TotalCount:    totalCount,
		PageSize:      pageSize,
		PrevPage:      maxInt(1, currentPage-1),
		NextPage:      minInt(totalPages, currentPage+1),
		PageNumbers:   visiblePages(currentPage, totalPages, 5),
	}
	s.render(w, "dashboard", data, http.StatusOK)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize)
	reader, err := r.MultipartReader()
	if err != nil {
		s.respondAdminAction(w, r, 1, http.StatusBadRequest, flashData{Message: "上传文件失败：文件过大或表单格式错误"}, false)
		return
	}

	currentPage := 1
	sharePassword := ""
	expiresAtValue := ""
	maxDownloadsValue := ""
	var fileName string
	var path string
	var mimeType string
	var shaValue string
	var size int64
	fileChosen := false

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if path != "" {
				_ = s.storage.Remove(path)
			}
			s.respondAdminAction(w, r, currentPage, http.StatusBadRequest, flashData{Message: "上传文件失败：表单读取异常"}, false)
			return
		}

		func() {
			defer part.Close()

			switch part.FormName() {
			case "page":
				currentPage = parsePositiveInt(readMultipartValue(part, 32), 1)
			case "share_password":
				sharePassword = strings.TrimSpace(readMultipartValue(part, 1024))
			case "expires_at":
				expiresAtValue = readMultipartValue(part, 64)
			case "max_downloads":
				maxDownloadsValue = readMultipartValue(part, 32)
			case "file":
				if fileChosen || strings.TrimSpace(part.FileName()) == "" {
					return
				}
				fileName = part.FileName()
				path, mimeType, shaValue, size, err = s.storage.SaveUploadedFile(part, fileName)
				if err == nil {
					fileChosen = true
				}
			}
		}()
		if err != nil {
			if path != "" {
				_ = s.storage.Remove(path)
			}
			s.respondAdminAction(w, r, currentPage, http.StatusInternalServerError, flashData{Message: "上传文件失败：无法保存文件"}, false)
			return
		}
	}

	if !fileChosen {
		slog.Warn("上传被取消或未选择文件", "请求ID", requestIDFromContext(r))
		s.respondAdminAction(w, r, currentPage, http.StatusBadRequest, flashData{Message: "上传文件失败：未选择文件"}, false)
		return
	}

	passwordHash, err := s.hashOptionalPassword(sharePassword)
	if err != nil {
		_ = s.storage.Remove(path)
		s.respondAdminAction(w, r, currentPage, http.StatusBadRequest, flashData{Message: "上传文件失败：密码处理异常"}, false)
		return
	}
	expiresAt, err := parseOptionalDateTimeLocal(expiresAtValue)
	if err != nil {
		_ = s.storage.Remove(path)
		s.respondAdminAction(w, r, currentPage, http.StatusBadRequest, flashData{Message: "上传文件失败：过期时间格式错误"}, false)
		return
	}
	maxDownloads, err := parseNonNegativeInt(maxDownloadsValue)
	if err != nil {
		_ = s.storage.Remove(path)
		s.respondAdminAction(w, r, currentPage, http.StatusBadRequest, flashData{Message: "上传文件失败：下载次数限制格式错误"}, false)
		return
	}

	item := model.Item{
		Kind:        "file",
		Name:        fileName,
		StoragePath: path,
		MIMEType:    mimeType,
		Ext:         strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), "."),
		Size:        size,
		SHA256:      shaValue,
	}
	summary, err := s.repo.CreateItemWithShare(r.Context(), item, passwordHash, sharePassword, expiresAt, maxDownloads)
	if err != nil {
		_ = s.storage.Remove(path)
		slog.Error("上传文件后创建分享失败",
			"请求ID", requestIDFromContext(r),
			"文件名", fileName,
			"错误", err,
		)
		s.respondAdminAction(w, r, currentPage, http.StatusInternalServerError, flashData{Message: "上传文件失败：数据库写入异常"}, false)
		return
	}
	slog.Info("文件上传成功",
		"请求ID", requestIDFromContext(r),
		"资源ID", summary.ID,
		"文件名", fileName,
		"大小", size,
		"过期时间", formatTimePtr(expiresAt),
		"下载限制", maxDownloads,
	)

	s.respondAdminAction(w, r, currentPage, http.StatusOK, s.buildSuccessFlash(r, summary, sharePassword, "文件已上传并生成分享链接"), true)
}

func (s *Server) handleCreateText(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：表单格式错误"})
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	content := r.FormValue("content")
	if name == "" || strings.TrimSpace(content) == "" {
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：标题和内容不能为空"})
		return
	}

	sharePassword := strings.TrimSpace(r.FormValue("share_password"))
	passwordHash, err := s.hashOptionalPassword(sharePassword)
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：密码处理异常"})
		return
	}
	expiresAt, err := parseOptionalDateTimeLocal(r.FormValue("expires_at"))
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：过期时间格式错误"})
		return
	}
	maxDownloads, err := parseNonNegativeInt(r.FormValue("max_downloads"))
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：下载次数限制格式错误"})
		return
	}

	mimeType := "text/plain; charset=utf-8"
	if strings.EqualFold(filepath.Ext(name), ".md") {
		mimeType = "text/markdown; charset=utf-8"
	}

	item := model.Item{
		Kind:        "text",
		Name:        name,
		ContentText: content,
		MIMEType:    mimeType,
		Ext:         strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."),
		Size:        int64(len([]byte(content))),
	}
	summary, err := s.repo.CreateItemWithShare(r.Context(), item, passwordHash, sharePassword, expiresAt, maxDownloads)
	if err != nil {
		slog.Error("创建文本分享失败",
			"请求ID", requestIDFromContext(r),
			"名称", name,
			"错误", err,
		)
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：数据库写入异常"})
		return
	}
	slog.Info("文本分享已创建",
		"请求ID", requestIDFromContext(r),
		"资源ID", summary.ID,
		"名称", name,
		"大小", item.Size,
		"过期时间", formatTimePtr(expiresAt),
		"下载限制", maxDownloads,
	)

	s.redirectAdminMessage(w, r, s.buildSuccessFlash(r, summary, sharePassword, "文本已保存并生成分享链接"))
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || itemID <= 0 {
		s.redirectAdminMessage(w, r, flashData{Message: "删除失败：资源 ID 无效"})
		return
	}

	item, err := s.repo.DeleteItem(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.redirectAdminMessage(w, r, flashData{Message: "删除失败：资源不存在"})
			return
		}
		s.redirectAdminMessage(w, r, flashData{Message: "删除失败：数据库异常"})
		return
	}

	if item.StoragePath != "" {
		_ = s.storage.Remove(item.StoragePath)
	}
	slog.Info("资源已删除",
		"请求ID", requestIDFromContext(r),
		"资源ID", item.ID,
		"名称", item.Name,
		"类型", item.Kind,
	)
	s.redirectAdminMessage(w, r, flashData{Message: "资源已删除"})
}

func (s *Server) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "批量删除失败：请求格式错误"})
		return
	}

	rawIDs := r.Form["item_ids"]
	if len(rawIDs) == 0 {
		s.redirectAdminMessage(w, r, flashData{Message: "批量删除失败：未选择任何资源"})
		return
	}

	itemIDs := make([]int64, 0, len(rawIDs))
	seen := make(map[int64]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		itemID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || itemID <= 0 {
			continue
		}
		if _, exists := seen[itemID]; exists {
			continue
		}
		seen[itemID] = struct{}{}
		itemIDs = append(itemIDs, itemID)
	}

	if len(itemIDs) == 0 {
		s.redirectAdminMessage(w, r, flashData{Message: "批量删除失败：资源 ID 无效"})
		return
	}

	items, err := s.repo.DeleteItems(r.Context(), itemIDs)
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "批量删除失败：数据库异常"})
		return
	}

	for _, item := range items {
		if item.StoragePath != "" {
			_ = s.storage.Remove(item.StoragePath)
		}
	}
	slog.Info("批量删除完成",
		"请求ID", requestIDFromContext(r),
		"数量", len(items),
	)

	s.redirectAdminMessage(w, r, flashData{Message: fmt.Sprintf("已批量删除 %d 个资源", len(items))})
}

func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	share, err := s.loadShare(r)
	if err != nil {
		s.renderShareError(w, err)
		return
	}
	if s.shareExpired(share) {
		s.logAccess(r, share, "share_view", "denied", "share expired")
		s.renderShareBlocked(w, share, "分享已过期", true, false)
		return
	}

	if share.PasswordHash != "" && !s.hasShareAccess(r, share) {
		if s.tryShareQueryAccess(w, r, share) {
			return
		}
		s.renderShareLocked(w, share, "")
		return
	}
	if s.downloadLimitReached(share) {
		s.logAccess(r, share, "share_view", "denied", "download limit reached")
		s.renderShareBlocked(w, share, "下载次数已用尽，当前分享不再提供预览。", false, true)
		return
	}

	s.logAccess(r, share, "share_view", "success", "")
	s.renderShareContent(w, r, share, "")
}

func (s *Server) handleShareVerify(w http.ResponseWriter, r *http.Request) {
	share, err := s.loadShare(r)
	if err != nil {
		s.renderShareError(w, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.logAccess(r, share, "password_verify", "error", "invalid form")
		s.renderShareLocked(w, share, "请求格式错误")
		return
	}

	password := r.FormValue("password")
	if !security.VerifyPassword(share.PasswordHash, password) {
		s.logAccess(r, share, "password_verify", "denied", "wrong password")
		s.renderShareLocked(w, share, "密码错误")
		return
	}

	s.logAccess(r, share, "password_verify", "success", "")
	s.setShareCookie(w, r, share)
	http.Redirect(w, r, "/s/"+share.ShareCode, http.StatusSeeOther)
}

func (s *Server) handleShareRaw(w http.ResponseWriter, r *http.Request) {
	share, err := s.loadShare(r)
	if err != nil {
		s.renderShareError(w, err)
		return
	}
	if s.shareExpired(share) {
		s.logAccess(r, share, "preview", "denied", "share expired")
		http.Error(w, "分享已过期", http.StatusGone)
		return
	}
	if share.PasswordHash != "" && !s.hasShareAccess(r, share) {
		s.logAccess(r, share, "preview", "denied", "password required")
		http.Error(w, "需要先验证分享密码", http.StatusUnauthorized)
		return
	}
	if s.downloadLimitReached(share) {
		s.logAccess(r, share, "preview", "denied", "download limit reached")
		http.Error(w, "下载次数已用尽，无法继续预览", http.StatusForbidden)
		return
	}

	prepared, err := s.prepareContent(share)
	if err != nil {
		s.handleContentPrepareError(w, r, share, err, "preview")
		return
	}
	defer prepared.close()

	s.logAccess(r, share, "preview", "success", "")
	s.serveItemContentPrepared(w, r, share, false, prepared)
}

func (s *Server) handleShareDownload(w http.ResponseWriter, r *http.Request) {
	share, err := s.loadShare(r)
	if err != nil {
		s.renderShareError(w, err)
		return
	}
	if s.shareExpired(share) {
		s.logAccess(r, share, "download", "denied", "share expired")
		http.Error(w, "分享已过期", http.StatusGone)
		return
	}
	if share.PasswordHash != "" && !s.hasShareAccess(r, share) {
		s.logAccess(r, share, "download", "denied", "password required")
		http.Error(w, "需要先验证分享密码", http.StatusUnauthorized)
		return
	}
	prepared, err := s.prepareContent(share)
	if err != nil {
		s.handleContentPrepareError(w, r, share, err, "download")
		return
	}
	defer prepared.close()
	ok, err := s.repo.IncrementDownloadCount(r.Context(), share.ID)
	if err != nil {
		s.logAccess(r, share, "download", "error", "increment download count failed")
		http.Error(w, "下载失败", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.logAccess(r, share, "download", "denied", "download limit reached")
		http.Error(w, "下载次数已用尽", http.StatusForbidden)
		return
	}
	share.DownloadCount++

	s.logAccess(r, share, "download", "success", "")
	s.serveItemContentPrepared(w, r, share, true, prepared)
}

func (s *Server) handleQRCodeImage(w http.ResponseWriter, r *http.Request) {
	value := strings.TrimSpace(r.URL.Query().Get("data"))
	if value == "" {
		http.Error(w, "二维码内容不能为空", http.StatusBadRequest)
		return
	}
	if len(value) > 4096 {
		http.Error(w, "二维码内容过长", http.StatusBadRequest)
		return
	}

	size := parseBoundedInt(r.URL.Query().Get("size"), 320, 96, 1024)
	png, err := qrcode.Encode(value, qrcode.Medium, size)
	if err != nil {
		http.Error(w, "生成二维码失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) serveItemContent(w http.ResponseWriter, r *http.Request, item model.SharedItem, download bool) {
	prepared, err := s.prepareContent(item)
	if err != nil {
		s.handleContentPrepareError(w, r, item, err, "preview")
		return
	}
	defer prepared.close()
	s.serveItemContentPrepared(w, r, item, download, prepared)
}

func (s *Server) serveItemContentPrepared(w http.ResponseWriter, r *http.Request, item model.SharedItem, download bool, prepared *preparedContent) {
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	mode := preview.Detect(item.Kind, item.MIMEType, item.Name)
	if !download && (mode == preview.ModeNone || mode == preview.ModeArchive) {
		disposition = "attachment"
	}
	if !download && strings.EqualFold(item.Ext, "svg") {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; script-src 'none'; object-src 'none'; base-uri 'none'; style-src 'unsafe-inline'; img-src data: blob: 'self'")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	filename := sanitizeFilename(item.Name)
	if download && item.Kind == "text" {
		filename = ensureTextDownloadName(item)
	}
	w.Header().Set("Content-Disposition", buildContentDisposition(disposition, filename))

	if item.Kind == "text" {
		if download && item.MIMEType != "" {
			w.Header().Set("Content-Type", item.MIMEType)
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		_, _ = io.WriteString(w, item.ContentText)
		return
	}

	if !download && mode == preview.ModeText {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else if item.MIMEType != "" {
		w.Header().Set("Content-Type", item.MIMEType)
	}
	if prepared == nil || prepared.file == nil {
		http.Error(w, "文件不存在", http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, item.Name, prepared.modTime, prepared.file)
}

func (s *Server) renderShareContent(w http.ResponseWriter, r *http.Request, item model.SharedItem, errText string) {
	mode := preview.Detect(item.Kind, item.MIMEType, item.Name)
	textHTML := template.HTML("")
	markdownHTML := template.HTML("")
	var archivePreview *preview.ArchiveSummary
	truncated := false
	isMarkdown := preview.IsMarkdown(item.Name, item.MIMEType)
	codeLanguage := preview.CodeLanguage(item.Name, item.MIMEType)
	copyTextSource := ""

	if mode == preview.ModeText {
		var err error
		var textPreview string
		textPreview, truncated, err = s.loadTextPreview(item)
		if err != nil {
			errText = "加载预览内容失败"
		} else if isMarkdown {
			markdownHTML = preview.RenderMarkdown(textPreview)
		} else {
			textHTML = preview.RenderCodeHTML(textPreview, codeLanguage)
		}
		copyTextSource = textPreview
		if item.Kind == "text" && item.ContentText != "" {
			copyTextSource = item.ContentText
		}
	} else if mode == preview.ModeArchive {
		var err error
		archivePreview, err = s.loadArchivePreview(item)
		if err != nil {
			errText = "加载压缩包预览失败"
		}
	}

	data := sharePageData{
		SiteName:          s.cfg.SiteName,
		Item:              item,
		ShareURL:          s.baseURL(r) + "/s/" + item.ShareCode,
		Locked:            false,
		Error:             errText,
		Expired:           false,
		NoDownloads:       s.downloadLimitReached(item),
		CanCopyText:       mode == preview.ModeText,
		CopyTextSource:    copyTextSource,
		DownloadsLeft:     s.downloadsLeft(item),
		PreviewMode:       mode,
		TextHTML:          textHTML,
		MarkdownHTML:      markdownHTML,
		CodeThemeCSS:      preview.ThemeCSS(),
		ArchivePreview:    archivePreview,
		IsMarkdown:        isMarkdown,
		CodeLanguageLabel: preview.LanguageLabel(codeLanguage),
		Truncated:         truncated,
		PreviewLimit:      s.cfg.PreviewLimit,
		RawURL:            "/s/" + item.ShareCode + "/raw",
		DownloadURL:       "/s/" + item.ShareCode + "/download",
	}
	s.render(w, "share", data, http.StatusOK)
}

func (s *Server) renderShareLocked(w http.ResponseWriter, item model.SharedItem, errText string) {
	data := sharePageData{
		SiteName:      s.cfg.SiteName,
		Item:          item,
		Locked:        true,
		Error:         errText,
		Expired:       false,
		NoDownloads:   false,
		CanCopyText:   false,
		DownloadsLeft: s.downloadsLeft(item),
		PreviewLimit:  s.cfg.PreviewLimit,
	}
	s.render(w, "share", data, http.StatusOK)
}

func (s *Server) renderShareBlocked(w http.ResponseWriter, item model.SharedItem, errText string, expired, noDownloads bool) {
	data := sharePageData{
		SiteName:      s.cfg.SiteName,
		Item:          item,
		Error:         errText,
		Expired:       expired,
		NoDownloads:   noDownloads,
		CanCopyText:   false,
		DownloadsLeft: s.downloadsLeft(item),
		PreviewLimit:  s.cfg.PreviewLimit,
	}
	s.render(w, "share", data, http.StatusOK)
}

func (s *Server) renderShareError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "分享不存在", http.StatusNotFound)
	default:
		http.Error(w, "加载分享失败", http.StatusInternalServerError)
	}
}

func (s *Server) loadShare(r *http.Request) (model.SharedItem, error) {
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		return model.SharedItem{}, sql.ErrNoRows
	}

	item, err := s.repo.GetSharedItemByCode(r.Context(), code)
	if err != nil {
		return model.SharedItem{}, err
	}
	if !item.ShareEnabled {
		return model.SharedItem{}, sql.ErrNoRows
	}
	return item, nil
}

func (s *Server) loadTextPreview(item model.SharedItem) (string, bool, error) {
	if item.Kind == "text" {
		data := []byte(item.ContentText)
		if int64(len(data)) > s.cfg.PreviewLimit {
			return string(data[:s.cfg.PreviewLimit]), true, nil
		}
		return item.ContentText, false, nil
	}
	return s.storage.ReadText(item.StoragePath, s.cfg.PreviewLimit)
}

func (s *Server) loadArchivePreview(item model.SharedItem) (*preview.ArchiveSummary, error) {
	if item.Kind != "file" {
		return nil, fmt.Errorf("archive preview only supports files")
	}
	file, err := s.storage.Open(item.StoragePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return preview.InspectArchive(item.Name, file, item.Size)
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminAuthed(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) isAdminAuthed(r *http.Request) bool {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return false
	}
	return security.EqualString(cookie.Value, s.adminCookieValue())
}

func (s *Server) adminCookieValue() string {
	return security.SignToken(s.cfg.SessionSecret, "admin:"+s.cfg.AdminUser+":"+s.cfg.AdminPass)
}

func (s *Server) hasShareAccess(r *http.Request, item model.SharedItem) bool {
	if item.PasswordHash == "" {
		return true
	}

	cookie, err := r.Cookie(s.shareCookieName(item.ShareCode))
	if err != nil {
		return false
	}
	return security.EqualString(cookie.Value, s.shareCookieValue(item))
}

func (s *Server) tryShareQueryAccess(w http.ResponseWriter, r *http.Request, item model.SharedItem) bool {
	query := r.URL.Query()

	if token := strings.TrimSpace(query.Get("token")); token != "" && item.AccessToken != "" {
		if security.EqualString(token, item.AccessToken) {
			s.logAccess(r, item, "password_verify", "success", "token access")
			s.setShareCookie(w, r, item)
			http.Redirect(w, r, "/s/"+item.ShareCode, http.StatusSeeOther)
			return true
		}
	}

	password := strings.TrimSpace(query.Get("p"))
	if password == "" {
		password = strings.TrimSpace(query.Get("password"))
	}
	if password != "" && security.VerifyPassword(item.PasswordHash, password) {
		s.logAccess(r, item, "password_verify", "success", "password url access")
		s.setShareCookie(w, r, item)
		http.Redirect(w, r, "/s/"+item.ShareCode, http.StatusSeeOther)
		return true
	}

	return false
}

func (s *Server) shareCookieName(shareCode string) string {
	return "fs_share_" + shareCode
}

func (s *Server) shareCookieValue(item model.SharedItem) string {
	return security.SignToken(s.cfg.SessionSecret, "share:"+item.ShareCode+":"+item.PasswordHash)
}

func (s *Server) setShareCookie(w http.ResponseWriter, r *http.Request, item model.SharedItem) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.shareCookieName(item.ShareCode),
		Value:    s.shareCookieValue(item),
		Path:     "/s/" + item.ShareCode,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isHTTPSRequest(r),
	})
}

func (s *Server) shareExpired(item model.SharedItem) bool {
	return item.ShareExpiresAt != nil && time.Now().After(item.ShareExpiresAt.Local())
}

func (s *Server) downloadLimitReached(item model.SharedItem) bool {
	return item.MaxDownloads > 0 && item.DownloadCount >= item.MaxDownloads
}

func (s *Server) downloadsLeft(item model.SharedItem) int {
	if item.MaxDownloads <= 0 {
		return -1
	}

	left := item.MaxDownloads - item.DownloadCount
	if left < 0 {
		return 0
	}
	return left
}

func (s *Server) hashOptionalPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	return security.HashPassword(password)
}

func (s *Server) redirectAdminMessage(w http.ResponseWriter, r *http.Request, flash flashData) {
	s.redirectAdminMessageAtPage(w, r, currentAdminPage(r), flash)
}

func (s *Server) redirectAdminMessageAtPage(w http.ResponseWriter, r *http.Request, page int, flash flashData) {
	s.writeFlash(w, r, flash)
	http.Redirect(w, r, s.adminPageTarget(page), http.StatusSeeOther)
}

func (s *Server) adminPageTarget(page int) string {
	target := "/admin"
	if page > 1 {
		target += "?page=" + strconv.Itoa(page)
	}
	return target
}

func (s *Server) respondAdminAction(w http.ResponseWriter, r *http.Request, page, status int, flash flashData, includeFlash bool) {
	if s.wantsJSON(r) {
		resp := actionResponse{
			OK:       status < 400,
			Message:  flash.Message,
			Redirect: s.adminPageTarget(page),
		}
		if includeFlash {
			flashCopy := flash
			resp.Flash = &flashCopy
		}
		s.writeJSON(w, status, resp)
		return
	}
	s.redirectAdminMessageAtPage(w, r, page, flash)
}

func (s *Server) render(w http.ResponseWriter, name string, data any, status int) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "模板渲染失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "JSON 响应失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL
	}

	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}

	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host
}

func humanSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

func formatUnix(unix int64) string {
	return time.Unix(unix, 0).Local().Format("2006-01-02 15:04")
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	if name == "" {
		return "download.bin"
	}
	return name
}

func ensureTextDownloadName(item model.SharedItem) string {
	name := sanitizeFilename(item.Name)
	if filepath.Ext(name) != "" {
		return name
	}

	if strings.Contains(strings.ToLower(item.MIMEType), "markdown") || strings.EqualFold(item.Ext, "md") {
		return name + ".md"
	}
	return name + ".txt"
}

func buildContentDisposition(disposition, filename string) string {
	fallback := asciiFilenameFallback(filename)
	if fallback == "" {
		fallback = "download"
	}

	if !utf8.ValidString(filename) {
		filename = fallback
	}

	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`,
		disposition,
		escapeContentDispositionValue(fallback),
		url.PathEscape(filename),
	)
}

func asciiFilenameFallback(name string) string {
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r == '"' || r == '\\' || r == '\r' || r == '\n':
			builder.WriteByte('_')
		case r >= 0x20 && r <= 0x7e:
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	fallback := strings.TrimSpace(builder.String())
	fallback = strings.Trim(fallback, ".")
	if fallback == "" {
		ext := filepath.Ext(name)
		if ext != "" && isASCII(ext) {
			return "download" + ext
		}
		return "download"
	}
	return fallback
}

func escapeContentDispositionValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 0x7f {
			return false
		}
	}
	return true
}

func (s *Server) buildSuccessFlash(r *http.Request, summary model.ItemSummary, sharePassword, message string) flashData {
	baseURL := s.baseURL(r)
	shareURL := baseURL + "/s/" + summary.ShareCode

	flash := flashData{
		Message:  message,
		ShareURL: shareURL,
		AutoCopy: shareURL,
	}

	if summary.PasswordProtected {
		if summary.ShareAccessToken != "" {
			flash.DirectURL = shareURL + "?token=" + url.QueryEscape(summary.ShareAccessToken)
		}
		flash.SharePassword = sharePassword
		if sharePassword != "" {
			flash.PasswordURL = shareURL + "?p=" + url.QueryEscape(sharePassword)
			flash.AutoCopy = flash.PasswordURL
		} else if flash.DirectURL != "" {
			flash.AutoCopy = flash.DirectURL
		}
	}

	return flash
}

func (s *Server) envExample() string {
	return strings.Join([]string{
		"# 编辑项目根目录 .env 后重启服务",
		"FILESERVICE_SITE_NAME=" + s.cfg.SiteName,
		"FILESERVICE_ADDR=" + s.cfg.Addr,
		"FILESERVICE_BASE_URL=" + firstNonEmpty(s.cfg.BaseURL, "http://127.0.0.1:8080"),
		"FILESERVICE_DATA_DIR=" + s.cfg.DataDir,
		"FILESERVICE_ADMIN_USER=" + s.cfg.AdminUser,
		"FILESERVICE_ADMIN_PASS=change-me",
		"FILESERVICE_SESSION_SECRET=change-me",
		"FILESERVICE_MAX_UPLOAD_SIZE=" + strconv.FormatInt(s.cfg.MaxUploadSize, 10),
		"FILESERVICE_PREVIEW_LIMIT=" + strconv.FormatInt(s.cfg.PreviewLimit, 10),
		"FILESERVICE_PAGE_SIZE=" + strconv.Itoa(s.cfg.PageSize),
		"FILESERVICE_ACCESS_LOG_RETENTION=" + strconv.Itoa(s.cfg.AccessLogRetention),
	}, "\n")
}

func (s *Server) writeFlash(w http.ResponseWriter, r *http.Request, flash flashData) {
	payload, err := json.Marshal(flash)
	if err != nil {
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminFlashCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(payload),
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   120,
		Secure:   s.isHTTPSRequest(r),
	})
}

func (s *Server) readFlash(w http.ResponseWriter, r *http.Request) flashData {
	cookie, err := r.Cookie(adminFlashCookieName)
	if err != nil {
		return flashData{}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminFlashCookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Secure:   s.isHTTPSRequest(r),
	})

	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return flashData{}
	}

	var flash flashData
	if err := json.Unmarshal(raw, &flash); err != nil {
		return flashData{}
	}
	return flash
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseOptionalDateTimeLocal(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseNonNegativeInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid non-negative int")
	}
	return parsed, nil
}

func currentAdminPage(r *http.Request) int {
	if err := r.ParseForm(); err == nil {
		if page := parsePositiveInt(r.FormValue("page"), 0); page > 0 {
			return page
		}
	}
	return parsePositiveInt(r.URL.Query().Get("page"), 1)
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBoundedInt(value string, fallback, minValue, maxValue int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func visiblePages(currentPage, totalPages, window int) []int {
	if totalPages <= 0 {
		return []int{1}
	}
	if window < 1 {
		window = 1
	}

	start := maxInt(1, currentPage-window/2)
	end := minInt(totalPages, start+window-1)
	start = maxInt(1, end-window+1)

	pages := make([]int, 0, end-start+1)
	for page := start; page <= end; page++ {
		pages = append(pages, page)
	}
	return pages
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) logAccess(r *http.Request, item model.SharedItem, eventType, status, message string) {
	if err := s.repo.CreateAccessLog(r.Context(), model.AccessLog{
		ShareCode: item.ShareCode,
		ItemName:  item.Name,
		EventType: eventType,
		Status:    status,
		Message:   message,
		ClientIP:  clientIP(r),
		UserAgent: firstNonEmpty(strings.TrimSpace(r.UserAgent()), "-"),
	}); err != nil {
		slog.Error("写入访问日志失败",
			"请求ID", requestIDFromContext(r),
			"分享码", item.ShareCode,
			"事件", eventType,
			"状态", status,
			"错误", err,
		)
		return
	}

	if status == "denied" || status == "error" {
		slog.Warn("分享访问受限",
			"请求ID", requestIDFromContext(r),
			"分享码", item.ShareCode,
			"资源", item.Name,
			"事件", eventType,
			"状态", status,
			"说明", message,
			"来源IP", clientIP(r),
		)
	}
}

func clientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		if first, _, ok := strings.Cut(forwardedFor, ","); ok {
			return strings.TrimSpace(first)
		}
		return forwardedFor
	}

	if index := strings.LastIndex(r.RemoteAddr, ":"); index > 0 {
		return r.RemoteAddr[:index]
	}
	return r.RemoteAddr
}

type ctxKey string

const requestIDKey ctxKey = "request_id"

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (lrw *loggingResponseWriter) WriteHeader(status int) {
	lrw.status = status
	lrw.ResponseWriter.WriteHeader(status)
}

func (lrw *loggingResponseWriter) Write(data []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}
	n, err := lrw.ResponseWriter.Write(data)
	lrw.bytes += n
	return n, err
}

func (s *Server) withMiddlewares(next http.Handler) http.Handler {
	return s.recoverMiddleware(s.requestLogMiddleware(next))
}

func (s *Server) requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := s.nextRequestID()
		ctx := withRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)

		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w}
		lrw.Header().Set("X-Request-Id", requestID)

		next.ServeHTTP(lrw, r)

		status := lrw.status
		if status == 0 {
			status = http.StatusOK
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		slog.Log(ctx, level, "请求完成",
			"请求ID", requestID,
			"方法", r.Method,
			"路径", requestPath(r),
			"状态码", status,
			"耗时毫秒", time.Since(start).Milliseconds(),
			"来源IP", clientIP(r),
		)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("服务内部异常",
					"请求ID", requestIDFromContext(r),
					"方法", r.Method,
					"路径", requestPath(r),
					"来源IP", clientIP(r),
					"异常", recovered,
				)
				http.Error(w, "服务器内部错误", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) nextRequestID() string {
	return fmt.Sprintf("req-%06d", s.reqSeq.Add(1))
}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func requestIDFromContext(r *http.Request) string {
	if value, ok := r.Context().Value(requestIDKey).(string); ok && value != "" {
		return value
	}
	return "-"
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04")
}

func requestPath(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func (p *preparedContent) close() {
	if p == nil || p.file == nil {
		return
	}
	_ = p.file.Close()
}

func (s *Server) prepareContent(item model.SharedItem) (*preparedContent, error) {
	if item.Kind == "text" {
		return &preparedContent{}, nil
	}

	file, err := s.storage.Open(item.StoragePath)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &preparedContent{
		file:    file,
		modTime: stat.ModTime(),
	}, nil
}

func (s *Server) handleContentPrepareError(w http.ResponseWriter, r *http.Request, item model.SharedItem, err error, eventType string) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.logAccess(r, item, eventType, "error", "file missing")
		http.Error(w, "文件不存在", http.StatusNotFound)
	default:
		s.logAccess(r, item, eventType, "error", "open content failed")
		http.Error(w, "读取文件失败", http.StatusInternalServerError)
	}
}

func (s *Server) isHTTPSRequest(r *http.Request) bool {
	if r == nil {
		return strings.HasPrefix(strings.ToLower(s.cfg.BaseURL), "https://")
	}
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(s.cfg.BaseURL), "https://")
}

func (s *Server) wantsJSON(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Requested-With")), "XMLHttpRequest") {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

func readMultipartValue(part *multipart.Part, limit int64) string {
	data, err := io.ReadAll(io.LimitReader(part, limit))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
