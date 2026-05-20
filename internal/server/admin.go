package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"file-service/internal/config"
	"file-service/internal/model"
)

const (
	settingMaxUploadSize      = "max_upload_size"
	settingPreviewLimit       = "preview_limit"
	settingPageSize           = "page_size"
	settingAccessLogRetention = "access_log_retention"
	settingDefaultExpire      = "default_expire_option"
)

type adminNavItem struct {
	Label  string
	URL    string
	Active bool
}

type adminStatCard struct {
	Label string
	Value string
	Note  string
}

type adminExpireOption struct {
	Value   string
	Label   string
	Checked bool
}

type adminPaginationLink struct {
	Label    string
	URL      string
	Current  bool
	Disabled bool
}

type adminPagination struct {
	CurrentPage int
	TotalPages  int
	TotalCount  int
	PageSize    int
	Links       []adminPaginationLink
}

type adminItemView struct {
	model.ItemSummary
	ShareURL           string
	PasswordURL        string
	DirectURL          string
	ExpiryLabel        string
	DownloadsLabel     string
	RemainingLabel     string
	VisibilityLabel    string
	VisibilityClass    string
	ShareStatusLabel   string
	ShareStatusClass   string
	IsExpired          bool
	DownloadsExhausted bool
	EditExpireOptions  []adminExpireOption
	EditExpireValue    string
	DownloadPolicy     string
	RemainingDownloads int
}

type adminFileSection struct {
	Items         []adminItemView
	Pagination    adminPagination
	Query         string
	Kind          string
	Status        string
	ShareMode     string
	CurrentURL    string
	SearchEmpty   bool
	MaxUploadSize int64
}

type adminShareSection struct {
	Items        []adminItemView
	Pagination   adminPagination
	Query        string
	Status       string
	CurrentURL   string
	DownloadLogs []model.AccessLog
}

type adminDashboardSection struct {
	Stats            model.DashboardStats
	StatCards        []adminStatCard
	RecentItems      []adminItemView
	RecentDownloads  []model.AccessLog
	RecentAccessLogs []model.AccessLog
}

type adminSettingsSection struct {
	Form config.RuntimeSettings
}

type adminPageData struct {
	SiteName       string
	BaseURL        string
	CurrentSection string
	Message        string
	Flash          flashData
	Nav            []adminNavItem
	ExpireOptions  []adminExpireOption
	Files          adminFileSection
	Shares         adminShareSection
	Dashboard      adminDashboardSection
	Settings       adminSettingsSection
}

type expireOptionDef struct {
	Value    string
	Label    string
	Duration time.Duration
}

var expireOptions = []expireOptionDef{
	{Value: "7h", Label: "7小时", Duration: 7 * time.Hour},
	{Value: "6h", Label: "6小时", Duration: 6 * time.Hour},
	{Value: "24h", Label: "24小时", Duration: 24 * time.Hour},
	{Value: "7d", Label: "7天", Duration: 7 * 24 * time.Hour},
	{Value: "30d", Label: "30天", Duration: 30 * 24 * time.Hour},
	{Value: "365d", Label: "365天", Duration: 365 * 24 * time.Hour},
	{Value: "never", Label: "永不过期"},
}

func (s *Server) handleAdminRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadRuntimeSettings(r.Context())
	if err != nil {
		http.Error(w, "加载系统配置失败", http.StatusInternalServerError)
		return
	}

	stats, err := s.repo.GetDashboardStats(r.Context())
	if err != nil {
		http.Error(w, "加载仪表盘统计失败", http.StatusInternalServerError)
		return
	}
	recentItems, err := s.repo.ListRecentItems(r.Context(), 8)
	if err != nil {
		http.Error(w, "加载最近资源失败", http.StatusInternalServerError)
		return
	}
	recentDownloads, err := s.repo.ListAccessLogs(r.Context(), "download", 12)
	if err != nil {
		http.Error(w, "加载下载记录失败", http.StatusInternalServerError)
		return
	}
	recentAccessLogs, err := s.repo.ListRecentAccessLogs(r.Context(), 12)
	if err != nil {
		http.Error(w, "加载访问日志失败", http.StatusInternalServerError)
		return
	}

	flash := s.readFlash(w, r)
	data := s.newAdminPageData(r, "dashboard", flash, settings)
	data.Dashboard = adminDashboardSection{
		Stats: stats,
		StatCards: []adminStatCard{
			{Label: "总资源数", Value: strconv.Itoa(stats.TotalItems), Note: fmt.Sprintf("文件 %d / 文本 %d", stats.FileItems, stats.TextItems)},
			{Label: "分享总数", Value: strconv.Itoa(stats.TotalShares), Note: fmt.Sprintf("有效 %d / 过期 %d", stats.ActiveShares, stats.ExpiredShares)},
			{Label: "累计下载", Value: strconv.Itoa(stats.TotalDownloads), Note: "统计成功下载次数"},
			{Label: "今日下载", Value: strconv.Itoa(stats.TodayDownloads), Note: "按本地时区当日计算"},
		},
		RecentItems:      s.mapAdminItems(r, recentItems),
		RecentDownloads:  recentDownloads,
		RecentAccessLogs: recentAccessLogs,
	}
	s.render(w, "admin", data, http.StatusOK)
}

func (s *Server) handleAdminFiles(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadRuntimeSettings(r.Context())
	if err != nil {
		http.Error(w, "加载系统配置失败", http.StatusInternalServerError)
		return
	}

	currentPage := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := settings.PageSize
	query := model.ItemQuery{
		Keyword: strings.TrimSpace(r.URL.Query().Get("q")),
		Kind:    strings.TrimSpace(r.URL.Query().Get("kind")),
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		Offset:  (currentPage - 1) * pageSize,
		Limit:   pageSize,
	}

	items, totalCount, err := s.repo.ListItemsPage(r.Context(), query)
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
		query.Offset = (currentPage - 1) * pageSize
		items, totalCount, err = s.repo.ListItemsPage(r.Context(), query)
		if err != nil {
			http.Error(w, "加载资源列表失败", http.StatusInternalServerError)
			return
		}
	}

	flash := s.readFlash(w, r)
	currentURL := adminFilesURL(currentPage, query.Keyword, query.Kind, query.Status, strings.TrimSpace(r.URL.Query().Get("mode")))
	data := s.newAdminPageData(r, "files", flash, settings)
	data.Files = adminFileSection{
		Items:         s.mapAdminItems(r, items),
		Pagination:    buildAdminPagination("/admin/files", currentPage, totalPages, totalCount, pageSize, url.Values{"q": {query.Keyword}, "kind": {query.Kind}, "status": {query.Status}, "mode": {strings.TrimSpace(r.URL.Query().Get("mode"))}}),
		Query:         query.Keyword,
		Kind:          query.Kind,
		Status:        query.Status,
		ShareMode:     defaultString(strings.TrimSpace(r.URL.Query().Get("mode")), "file"),
		CurrentURL:    currentURL,
		SearchEmpty:   totalCount == 0,
		MaxUploadSize: settings.MaxUploadSize,
	}
	s.render(w, "admin", data, http.StatusOK)
}

func (s *Server) handleAdminShares(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadRuntimeSettings(r.Context())
	if err != nil {
		http.Error(w, "加载系统配置失败", http.StatusInternalServerError)
		return
	}

	currentPage := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := settings.PageSize
	query := model.ShareQuery{
		Keyword: strings.TrimSpace(r.URL.Query().Get("q")),
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		Offset:  (currentPage - 1) * pageSize,
		Limit:   pageSize,
	}

	items, totalCount, err := s.repo.ListSharesPage(r.Context(), query)
	if err != nil {
		http.Error(w, "加载分享列表失败", http.StatusInternalServerError)
		return
	}

	totalPages := maxInt(1, (totalCount+pageSize-1)/pageSize)
	if totalCount == 0 {
		totalPages = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
		query.Offset = (currentPage - 1) * pageSize
		items, totalCount, err = s.repo.ListSharesPage(r.Context(), query)
		if err != nil {
			http.Error(w, "加载分享列表失败", http.StatusInternalServerError)
			return
		}
	}

	downloadLogs, err := s.repo.ListAccessLogs(r.Context(), "download", 20)
	if err != nil {
		http.Error(w, "加载下载记录失败", http.StatusInternalServerError)
		return
	}

	flash := s.readFlash(w, r)
	currentURL := adminSharesURL(currentPage, query.Keyword, query.Status)
	data := s.newAdminPageData(r, "shares", flash, settings)
	data.Shares = adminShareSection{
		Items:        s.mapAdminItems(r, items),
		Pagination:   buildAdminPagination("/admin/shares", currentPage, totalPages, totalCount, pageSize, url.Values{"q": {query.Keyword}, "status": {query.Status}}),
		Query:        query.Keyword,
		Status:       query.Status,
		CurrentURL:   currentURL,
		DownloadLogs: downloadLogs,
	}
	s.render(w, "admin", data, http.StatusOK)
}

func (s *Server) handleUpdateShare(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：表单格式错误"})
		return
	}

	itemID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil || itemID <= 0 {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：资源 ID 无效"})
		return
	}

	summary, err := s.repo.GetShareSummaryByItemID(r.Context(), itemID)
	if err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：分享不存在"})
		return
	}

	expireOption := strings.TrimSpace(r.FormValue("expire_option"))
	expiresAt, err := resolveShareEditExpireOption(expireOption)
	if err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：过期时间选项无效"})
		return
	}

	downloadPolicy := strings.TrimSpace(r.FormValue("download_policy"))
	remainingDownloads, err := parseRemainingDownloads(r.FormValue("remaining_downloads"))
	if err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：剩余下载次数无效"})
		return
	}

	maxDownloads, err := resolveMaxDownloadsForUpdate(summary, downloadPolicy, remainingDownloads)
	if err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：下载策略无效"})
		return
	}

	if err := s.repo.UpdateShareSettings(r.Context(), itemID, expiresAt, maxDownloads); err != nil {
		s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "更新分享失败：数据库异常"})
		return
	}

	s.redirectAdminTarget(w, r, currentAdminTarget(r), flashData{Message: "分享设置已更新"})
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadRuntimeSettings(r.Context())
	if err != nil {
		http.Error(w, "加载系统配置失败", http.StatusInternalServerError)
		return
	}

	flash := s.readFlash(w, r)
	data := s.newAdminPageData(r, "settings", flash, settings)
	data.Settings = adminSettingsSection{
		Form: settings,
	}
	s.render(w, "admin", data, http.StatusOK)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：表单格式错误"})
		return
	}

	maxUploadSize, err := parsePositiveInt64(r.FormValue("max_upload_size"), config.DefaultMaxUploadSize)
	if err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：最大上传大小无效"})
		return
	}
	previewLimit, err := parsePositiveInt64(r.FormValue("preview_limit"), config.DefaultPreviewLimit)
	if err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：预览大小限制无效"})
		return
	}
	pageSize, err := parsePositiveIntValue(r.FormValue("page_size"), config.DefaultPageSize)
	if err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：分页大小无效"})
		return
	}
	logRetention, err := parsePositiveIntValue(r.FormValue("access_log_retention"), config.DefaultAccessLogRetention)
	if err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：日志保留条数无效"})
		return
	}

	defaultExpireOption := strings.TrimSpace(r.FormValue("default_expire_option"))
	if _, err := resolveExpireOption(defaultExpireOption); err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：默认过期选项无效"})
		return
	}

	err = s.repo.UpdateSystemSettings(r.Context(), map[string]string{
		settingMaxUploadSize:      strconv.FormatInt(maxUploadSize, 10),
		settingPreviewLimit:       strconv.FormatInt(previewLimit, 10),
		settingPageSize:           strconv.Itoa(pageSize),
		settingAccessLogRetention: strconv.Itoa(logRetention),
		settingDefaultExpire:      defaultExpireOption,
	})
	if err != nil {
		s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "保存配置失败：数据库异常"})
		return
	}

	s.redirectAdminTarget(w, r, "/admin/settings", flashData{Message: "系统配置已保存"})
}

func (s *Server) newAdminPageData(r *http.Request, section string, flash flashData, settings config.RuntimeSettings) adminPageData {
	return adminPageData{
		SiteName:       s.cfg.SiteName,
		BaseURL:        s.baseURL(r),
		CurrentSection: section,
		Message:        flash.Message,
		Flash:          flash,
		Nav: []adminNavItem{
			{Label: "仪表盘", URL: "/admin/dashboard", Active: section == "dashboard"},
			{Label: "文件", URL: "/admin/files", Active: section == "files"},
			{Label: "分享", URL: "/admin/shares", Active: section == "shares"},
			{Label: "配置", URL: "/admin/settings", Active: section == "settings"},
		},
		ExpireOptions: expireOptionsForValue(settings.DefaultExpireOption),
	}
}

func (s *Server) ensureRuntimeSettingsDefaults(ctx context.Context) error {
	defaults := map[string]string{
		settingMaxUploadSize:      strconv.FormatInt(s.cfg.DefaultMaxUploadSize, 10),
		settingPreviewLimit:       strconv.FormatInt(s.cfg.DefaultPreviewLimit, 10),
		settingPageSize:           strconv.Itoa(s.cfg.DefaultPageSize),
		settingAccessLogRetention: strconv.Itoa(s.cfg.AccessLogRetention),
		settingDefaultExpire:      config.DefaultExpireOption,
	}
	return s.repo.EnsureSystemSettings(ctx, defaults)
}

func (s *Server) loadRuntimeSettings(ctx context.Context) (config.RuntimeSettings, error) {
	settings := config.DefaultRuntimeConfig()
	values, err := s.repo.GetSystemSettings(ctx, []string{
		settingMaxUploadSize,
		settingPreviewLimit,
		settingPageSize,
		settingAccessLogRetention,
		settingDefaultExpire,
	})
	if err != nil {
		return settings, err
	}

	settings.MaxUploadSize = parseStoredInt64(values[settingMaxUploadSize], s.cfg.DefaultMaxUploadSize)
	settings.PreviewLimit = parseStoredInt64(values[settingPreviewLimit], s.cfg.DefaultPreviewLimit)
	settings.PageSize = parseStoredInt(values[settingPageSize], s.cfg.DefaultPageSize)
	settings.AccessLogRetention = parseStoredInt(values[settingAccessLogRetention], s.cfg.AccessLogRetention)
	settings.DefaultExpireOption = defaultString(values[settingDefaultExpire], config.DefaultExpireOption)
	return settings, nil
}

func (s *Server) previewLimitForRequest(ctx context.Context) int64 {
	settings, err := s.loadRuntimeSettings(ctx)
	if err != nil {
		return s.cfg.DefaultPreviewLimit
	}
	return settings.PreviewLimit
}

func (s *Server) mapAdminItems(r *http.Request, items []model.ItemSummary) []adminItemView {
	baseURL := s.baseURL(r)
	result := make([]adminItemView, 0, len(items))
	for _, item := range items {
		downloadPolicy := "unlimited"
		remainingDownloads := 0
		view := adminItemView{
			ItemSummary:        item,
			ShareURL:           baseURL + "/s/" + item.ShareCode,
			ExpiryLabel:        "永不过期",
			DownloadsLabel:     "不限",
			RemainingLabel:     "不限",
			VisibilityLabel:    "公开",
			VisibilityClass:    "status-tag-soft",
			ShareStatusLabel:   "有效",
			ShareStatusClass:   "status-tag-soft",
			IsExpired:          s.shareExpired(model.SharedItem{ShareExpiresAt: item.ShareExpiresAt}),
			DownloadsExhausted: item.MaxDownloads > 0 && item.DownloadCount >= item.MaxDownloads,
			EditExpireOptions:  shareEditExpireOptions(item.ShareExpiresAt),
			EditExpireValue:    inferExpireOption(item.ShareExpiresAt),
		}
		if item.PasswordProtected {
			view.VisibilityLabel = "密码"
			view.VisibilityClass = ""
			if item.SharePassword != "" {
				view.PasswordURL = view.ShareURL + "?p=" + url.QueryEscape(item.SharePassword)
			}
			if item.ShareAccessToken != "" {
				view.DirectURL = view.ShareURL + "?token=" + url.QueryEscape(item.ShareAccessToken)
			}
		}
		if item.ShareExpiresAt != nil {
			view.ExpiryLabel = formatTimePtr(item.ShareExpiresAt)
		}
		if item.MaxDownloads < 0 {
			view.DownloadsLabel = "已禁用"
			view.RemainingLabel = "0"
			view.DownloadsExhausted = true
			downloadPolicy = "limited"
			remainingDownloads = 0
		} else if item.MaxDownloads > 0 {
			view.DownloadsLabel = fmt.Sprintf("%d / %d", item.DownloadCount, item.MaxDownloads)
			view.RemainingLabel = strconv.Itoa(maxInt(0, item.MaxDownloads-item.DownloadCount))
			downloadPolicy = "limited"
			remainingDownloads = maxInt(0, item.MaxDownloads-item.DownloadCount)
		} else {
			view.RemainingLabel = "不限"
		}
		if view.IsExpired {
			view.ShareStatusLabel = "已过期"
			view.ShareStatusClass = ""
		} else if view.DownloadsExhausted {
			view.ShareStatusLabel = "下载耗尽"
			view.ShareStatusClass = "status-tag"
		}
		view.DownloadPolicy = downloadPolicy
		view.RemainingDownloads = remainingDownloads
		result = append(result, view)
	}
	return result
}

func buildAdminPagination(base string, currentPage, totalPages, totalCount, pageSize int, params url.Values) adminPagination {
	pagination := adminPagination{
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		TotalCount:  totalCount,
		PageSize:    pageSize,
	}

	if params == nil {
		params = url.Values{}
	}
	prevPage := maxInt(1, currentPage-1)
	nextPage := minInt(totalPages, currentPage+1)
	pagination.Links = append(pagination.Links, adminPaginationLink{
		Label:    "上一页",
		URL:      buildAdminPageURL(base, params, prevPage),
		Disabled: currentPage <= 1,
	})
	for _, page := range visiblePages(currentPage, totalPages, 5) {
		pagination.Links = append(pagination.Links, adminPaginationLink{
			Label:   strconv.Itoa(page),
			URL:     buildAdminPageURL(base, params, page),
			Current: page == currentPage,
		})
	}
	pagination.Links = append(pagination.Links, adminPaginationLink{
		Label:    "下一页",
		URL:      buildAdminPageURL(base, params, nextPage),
		Disabled: currentPage >= totalPages,
	})
	return pagination
}

func buildAdminPageURL(base string, params url.Values, page int) string {
	cloned := url.Values{}
	for key, values := range params {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				cloned.Add(key, value)
			}
		}
	}
	if page > 1 {
		cloned.Set("page", strconv.Itoa(page))
	} else {
		cloned.Del("page")
	}
	encoded := cloned.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func adminFilesURL(page int, keyword, kind, status, mode string) string {
	return buildAdminPageURL("/admin/files", url.Values{
		"q":      {keyword},
		"kind":   {kind},
		"status": {status},
		"mode":   {mode},
	}, page)
}

func adminSharesURL(page int, keyword, status string) string {
	return buildAdminPageURL("/admin/shares", url.Values{
		"q":      {keyword},
		"status": {status},
	}, page)
}

func adminFilesTarget(r *http.Request) string {
	return sanitizeAdminTarget(r.URL.Query().Get("redirect_to"), "/admin/files")
}

func currentAdminTarget(r *http.Request) string {
	target := sanitizeAdminTarget(r.FormValue("redirect_to"), "")
	if target != "" {
		return target
	}
	target = sanitizeAdminTarget(r.URL.Query().Get("redirect_to"), "")
	if target != "" {
		return target
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/admin/shares"):
		return "/admin/shares"
	case strings.HasPrefix(r.URL.Path, "/admin/settings"):
		return "/admin/settings"
	default:
		return "/admin/files"
	}
}

func (s *Server) redirectAdminTarget(w http.ResponseWriter, r *http.Request, target string, flash flashData) {
	s.writeFlash(w, r, flash)
	http.Redirect(w, r, sanitizeAdminTarget(target, "/admin/dashboard"), http.StatusSeeOther)
}

func (s *Server) respondAdminActionToTarget(w http.ResponseWriter, r *http.Request, target string, status int, flash flashData, includeFlash bool) {
	target = sanitizeAdminTarget(target, "/admin/files")
	if s.wantsJSON(r) {
		if includeFlash {
			s.writeFlash(w, r, flash)
		}
		resp := actionResponse{
			OK:       status < 400,
			Message:  flash.Message,
			Redirect: target,
		}
		if includeFlash {
			flashCopy := flash
			resp.Flash = &flashCopy
		}
		s.writeJSON(w, status, resp)
		return
	}
	s.redirectAdminTarget(w, r, target, flash)
}

func sanitizeAdminTarget(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if !strings.HasPrefix(value, "/admin") {
		return fallback
	}
	return value
}

func expireOptionsForValue(selected string) []adminExpireOption {
	selected = defaultString(strings.TrimSpace(selected), config.DefaultExpireOption)
	options := make([]adminExpireOption, 0, len(expireOptions))
	for _, option := range expireOptions {
		options = append(options, adminExpireOption{
			Value:   option.Value,
			Label:   option.Label,
			Checked: option.Value == selected,
		})
	}
	return options
}

func shareEditExpireOptions(expiresAt *time.Time) []adminExpireOption {
	selected := inferExpireOption(expiresAt)
	options := make([]adminExpireOption, 0, len(expireOptions)+1)
	options = append(options, adminExpireOption{
		Value:   "expired_now",
		Label:   "立即过期",
		Checked: selected == "expired_now",
	})
	for _, option := range expireOptions {
		options = append(options, adminExpireOption{
			Value:   option.Value,
			Label:   option.Label,
			Checked: option.Value == selected,
		})
	}
	return options
}

func resolveExpireOption(value string) (*time.Time, error) {
	value = defaultString(strings.TrimSpace(value), config.DefaultExpireOption)
	now := time.Now()
	for _, option := range expireOptions {
		if option.Value != value {
			continue
		}
		if option.Value == "never" {
			return nil, nil
		}
		expiresAt := now.Add(option.Duration)
		return &expiresAt, nil
	}
	return nil, fmt.Errorf("invalid expire option")
}

func resolveShareEditExpireOption(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "expired_now" {
		expiredAt := time.Now().Add(-time.Minute)
		return &expiredAt, nil
	}
	return resolveExpireOption(value)
}

func inferExpireOption(expiresAt *time.Time) string {
	if expiresAt == nil {
		return "never"
	}
	now := time.Now()
	if expiresAt.Before(now) {
		return "expired_now"
	}

	remaining := expiresAt.Sub(now)
	bestValue := config.DefaultExpireOption
	bestDiff := time.Duration(1<<63 - 1)
	for _, option := range expireOptions {
		if option.Value == "never" {
			continue
		}
		diff := option.Duration - remaining
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			bestValue = option.Value
		}
	}
	return bestValue
}

func parseRemainingDownloads(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid remaining downloads")
	}
	return parsed, nil
}

func resolveMaxDownloadsForUpdate(summary model.ItemSummary, policy string, remaining int) (int, error) {
	switch strings.TrimSpace(policy) {
	case "", "unlimited":
		return 0, nil
	case "limited":
		if remaining == 0 {
			return -1, nil
		}
		return summary.DownloadCount + remaining, nil
	default:
		return 0, fmt.Errorf("invalid download policy")
	}
}

func parseStoredInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseStoredInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parsePositiveInt64(value string, fallback int64) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid positive int64")
	}
	return parsed, nil
}

func parsePositiveIntValue(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid positive int")
	}
	return parsed, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
