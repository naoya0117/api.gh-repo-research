package server

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/naoya0117/shuron2025/api/internal/database"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

type AdminServer struct {
	db         *database.DB
	templates  *template.Template
	resultOpts []string
}

type adminPageData struct {
	CheckQueries  []database.CheckQuery
	Repositories  []database.Repository
	MyChecks      []database.MyCheckSummary
	FlashMessage  string
	ErrorMessage  string
	ResultOptions []string
}

func NewAdminServer(db *database.DB) (*AdminServer, error) {
	tmpl, err := template.New("admin").Funcs(template.FuncMap{
		"boolLabel": func(v *bool) string {
			if v == nil {
				return "-"
			}
			if *v {
				return "True"
			}
			return "False"
		},
	}).ParseFS(templateFS, "templates/admin.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &AdminServer{
		db:         db,
		templates:  tmpl,
		resultOpts: []string{"○", "×", "△"},
	}, nil
}

func (s *AdminServer) HandleCheckQueries(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderAdminPage(w, r, "", "")
	case http.MethodPost:
		s.handleCreateCheckQuery(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *AdminServer) HandleCheckResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.handleCreateCheckResult(w, r)
}

func (s *AdminServer) handleCreateCheckQuery(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderAdminPage(w, r, "", "フォームの解析に失敗しました")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		s.renderAdminPage(w, r, "", "名前は必須です")
		return
	}

	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	if _, err := s.db.InsertCheckQuery(name, descPtr); err != nil {
		s.renderAdminPage(w, r, "", fmt.Sprintf("チェッククエリの登録に失敗しました: %v", err))
		return
	}

	http.Redirect(w, r, withFlash("/admin/check-queries", "チェッククエリを登録しました"), http.StatusSeeOther)
}

func (s *AdminServer) handleCreateCheckResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderAdminPage(w, r, "", "フォームの解析に失敗しました")
		return
	}

	repositoryID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("repository_id")))
	if err != nil || repositoryID <= 0 {
		s.renderAdminPage(w, r, "", "リポジトリIDが不正です")
		return
	}

	checkQueryID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("check_query_id")))
	if err != nil || checkQueryID <= 0 {
		s.renderAdminPage(w, r, "", "チェッククエリIDが不正です")
		return
	}

	result := strings.TrimSpace(r.FormValue("result"))
	if !s.isValidResult(result) {
		s.renderAdminPage(w, r, "", "結果は○, ×, △のいずれかを選択してください")
		return
	}

	memo := strings.TrimSpace(r.FormValue("memo"))
	var memoPtr *string
	if memo != "" {
		memoPtr = &memo
	}

	isWebAppStr := strings.TrimSpace(r.FormValue("is_web_app"))
	var isWebApp bool
	switch isWebAppStr {
	case "true":
		isWebApp = true
	case "false":
		isWebApp = false
	default:
		s.renderAdminPage(w, r, "", "Webアプリ判定はTrueかFalseを指定してください")
		return
	}

	if err := s.db.UpsertMyCheckedRepository(repositoryID, checkQueryID, result, memoPtr); err != nil {
		s.renderAdminPage(w, r, "", fmt.Sprintf("判定結果の保存に失敗しました: %v", err))
		return
	}

	if err := s.db.UpsertRepositoryWebAppCheck(repositoryID, isWebApp); err != nil {
		s.renderAdminPage(w, r, "", fmt.Sprintf("Webアプリ判定の保存に失敗しました: %v", err))
		return
	}

	http.Redirect(w, r, withFlash("/admin/check-queries", "判定結果を登録しました"), http.StatusSeeOther)
}

func (s *AdminServer) renderAdminPage(w http.ResponseWriter, r *http.Request, flash, errMsg string) {
	queries, err := s.db.GetCheckQueries()
	if err != nil {
		http.Error(w, fmt.Sprintf("チェッククエリの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	repos, err := s.db.GetRepositories(50, 0)
	if err != nil {
		http.Error(w, fmt.Sprintf("リポジトリ一覧の取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	checks, err := s.db.ListMyCheckSummaries(50)
	if err != nil {
		http.Error(w, fmt.Sprintf("判定履歴の取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	if flash == "" {
		flash = r.URL.Query().Get("flash")
	}
	if errMsg == "" {
		errMsg = r.URL.Query().Get("error")
	}

	data := adminPageData{
		CheckQueries:  queries,
		Repositories:  repos,
		MyChecks:      checks,
		FlashMessage:  flash,
		ErrorMessage:  errMsg,
		ResultOptions: s.resultOpts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "admin.gohtml", data); err != nil {
		http.Error(w, fmt.Sprintf("テンプレートの描画に失敗しました: %v", err), http.StatusInternalServerError)
	}
}

func (s *AdminServer) isValidResult(result string) bool {
	for _, opt := range s.resultOpts {
		if opt == result {
			return true
		}
	}
	return false
}

func withFlash(path, message string) string {
	return fmt.Sprintf("%s?flash=%s", path, url.QueryEscape(message))
}
