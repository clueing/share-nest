package server

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"file-service/internal/config"
	"file-service/internal/model"
	"file-service/internal/preview"
	"file-service/internal/repo"
	"file-service/internal/security"
	"file-service/internal/storage"
	"file-service/internal/ui"
)

const adminCookieName = "fs_admin_session"
const adminFlashCookieName = "fs_admin_flash"

type Server struct {
	cfg       config.Config
	repo      *repo.SQLiteRepo
	storage   *storage.Local
	templates *template.Template
	mux       *http.ServeMux
}

type dashboardData struct {
	SiteName   string
	BaseURL    string
	Items      []model.ItemSummary
	AccessLogs []model.AccessLog
	Message    string
	Flash      flashData
	EnvPath    string
	EnvExample string
	CurrentPage int
	TotalPages  int
	TotalCount  int
	PageSize    int
	PrevPage    int
	NextPage    int
	PageNumbers []int
}

type sharePageData struct {
	SiteName     string
	Item         model.SharedItem
	Locked       bool
	Error        string
	Expired      bool
	NoDownloads  bool
	PreviewMode  preview.Mode
	TextPreview  string
	Truncated    bool
	PreviewLimit int64
	RawURL       string
	DownloadURL  string
}

type flashData struct {
	Message       string `json:"message"`
	ShareURL      string `json:"share_url"`
	DirectURL     string `json:"direct_url"`
	PasswordURL   string `json:"password_url"`
	SharePassword string `json:"share_password"`
	AutoCopy      string `json:"auto_copy"`
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
	return s.mux
}

func (s *Server) routes(staticFS http.FileSystem) {
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	s.mux.Handle("GET /favicon.ico", http.FileServer(staticFS))

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
		SiteName:    s.cfg.SiteName,
		BaseURL:     s.baseURL(r),
		Items:       items,
		AccessLogs:  logs,
		Message:     flash.Message,
		Flash:       flash,
		EnvPath:     ".env",
		EnvExample:  s.envExample(),
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		TotalCount:  totalCount,
		PageSize:    pageSize,
		PrevPage:    maxInt(1, currentPage-1),
		NextPage:    minInt(totalPages, currentPage+1),
		PageNumbers: visiblePages(currentPage, totalPages, 5),
	}
	s.render(w, "dashboard", data, http.StatusOK)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadSize); err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：文件过大或表单格式错误"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：未选择文件"})
		return
	}

	passwordHash, err := s.hashOptionalPassword(strings.TrimSpace(r.FormValue("share_password")))
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：密码处理异常"})
		return
	}
	sharePassword := strings.TrimSpace(r.FormValue("share_password"))
	expiresAt, err := parseOptionalDateTimeLocal(r.FormValue("expires_at"))
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：过期时间格式错误"})
		return
	}
	maxDownloads, err := parseNonNegativeInt(r.FormValue("max_downloads"))
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：下载次数限制格式错误"})
		return
	}

	path, mimeType, shaValue, size, err := s.storage.SaveUploadedFile(file, header.Filename)
	if err != nil {
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：无法保存文件"})
		return
	}

	item := model.Item{
		Kind:        "file",
		Name:        header.Filename,
		StoragePath: path,
		MIMEType:    mimeType,
		Ext:         strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), "."),
		Size:        size,
		SHA256:      shaValue,
	}
	summary, err := s.repo.CreateItemWithShare(r.Context(), item, passwordHash, sharePassword, expiresAt, maxDownloads)
	if err != nil {
		_ = s.storage.Remove(path)
		s.redirectAdminMessage(w, r, flashData{Message: "上传文件失败：数据库写入异常"})
		return
	}

	s.redirectAdminMessage(w, r, s.buildSuccessFlash(r, summary, sharePassword, "文件已上传并生成分享链接"))
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
		s.redirectAdminMessage(w, r, flashData{Message: "创建文本失败：数据库写入异常"})
		return
	}

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
	s.setShareCookie(w, share)
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

	s.logAccess(r, share, "preview", "success", "")
	s.serveItemContent(w, r, share, false)
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
	s.serveItemContent(w, r, share, true)
}

func (s *Server) serveItemContent(w http.ResponseWriter, r *http.Request, item model.SharedItem, download bool) {
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, sanitizeFilename(item.Name)))

	if item.Kind == "text" {
		if item.MIMEType != "" {
			w.Header().Set("Content-Type", item.MIMEType)
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		_, _ = io.WriteString(w, item.ContentText)
		return
	}

	file, err := s.storage.Open(item.StoragePath)
	if err != nil {
		http.Error(w, "文件不存在", http.StatusNotFound)
		return
	}
	defer file.Close()

	if item.MIMEType != "" {
		w.Header().Set("Content-Type", item.MIMEType)
	}

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "读取文件失败", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, item.Name, stat.ModTime(), file)
}

func (s *Server) renderShareContent(w http.ResponseWriter, r *http.Request, item model.SharedItem, errText string) {
	mode := preview.Detect(item.Kind, item.MIMEType, item.Name)
	textPreview := ""
	truncated := false

	if mode == preview.ModeText {
		var err error
		textPreview, truncated, err = s.loadTextPreview(item)
		if err != nil {
			errText = "加载预览内容失败"
		}
	}

	data := sharePageData{
		SiteName:     s.cfg.SiteName,
		Item:         item,
		Locked:       false,
		Error:        errText,
		Expired:      false,
		NoDownloads:  s.downloadLimitReached(item),
		PreviewMode:  mode,
		TextPreview:  textPreview,
		Truncated:    truncated,
		PreviewLimit: s.cfg.PreviewLimit,
		RawURL:       "/s/" + item.ShareCode + "/raw",
		DownloadURL:  "/s/" + item.ShareCode + "/download",
	}
	s.render(w, "share", data, http.StatusOK)
}

func (s *Server) renderShareLocked(w http.ResponseWriter, item model.SharedItem, errText string) {
	data := sharePageData{
		SiteName:     s.cfg.SiteName,
		Item:         item,
		Locked:       true,
		Error:        errText,
		Expired:      false,
		NoDownloads:  false,
		PreviewLimit: s.cfg.PreviewLimit,
	}
	s.render(w, "share", data, http.StatusOK)
}

func (s *Server) renderShareBlocked(w http.ResponseWriter, item model.SharedItem, errText string, expired, noDownloads bool) {
	data := sharePageData{
		SiteName:     s.cfg.SiteName,
		Item:         item,
		Error:        errText,
		Expired:      expired,
		NoDownloads:  noDownloads,
		PreviewLimit: s.cfg.PreviewLimit,
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
			s.setShareCookie(w, item)
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
		s.setShareCookie(w, item)
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

func (s *Server) setShareCookie(w http.ResponseWriter, item model.SharedItem) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.shareCookieName(item.ShareCode),
		Value:    s.shareCookieValue(item),
		Path:     "/s/" + item.ShareCode,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) shareExpired(item model.SharedItem) bool {
	return item.ShareExpiresAt != nil && time.Now().After(item.ShareExpiresAt.Local())
}

func (s *Server) downloadLimitReached(item model.SharedItem) bool {
	return item.MaxDownloads > 0 && item.DownloadCount >= item.MaxDownloads
}

func (s *Server) hashOptionalPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	return security.HashPassword(password)
}

func (s *Server) redirectAdminMessage(w http.ResponseWriter, r *http.Request, flash flashData) {
	s.writeFlash(w, flash)
	target := "/admin"
	if page := currentAdminPage(r); page > 1 {
		target += "?page=" + strconv.Itoa(page)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
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
		"FILESERVICE_MAX_UPLOAD_SIZE=" + strconv.FormatInt(s.cfg.MaxUploadSize, 10),
		"FILESERVICE_PREVIEW_LIMIT=" + strconv.FormatInt(s.cfg.PreviewLimit, 10),
		"FILESERVICE_PAGE_SIZE=" + strconv.Itoa(s.cfg.PageSize),
	}, "\n")
}

func (s *Server) writeFlash(w http.ResponseWriter, flash flashData) {
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
	_ = s.repo.CreateAccessLog(r.Context(), model.AccessLog{
		ShareCode: item.ShareCode,
		ItemName:  item.Name,
		EventType: eventType,
		Status:    status,
		Message:   message,
		ClientIP:  clientIP(r),
		UserAgent: firstNonEmpty(strings.TrimSpace(r.UserAgent()), "-"),
	})
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
