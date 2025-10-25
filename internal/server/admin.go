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
	CheckQueries  []database.CheckQuery     `json:"checkQueries"`
	Repositories  []database.Repository     `json:"repositories"`
	MyChecks      []database.MyCheckSummary `json:"myChecks"`
	FlashMessage  string                    `json:"flashMessage"`
	ErrorMessage  string                    `json:"errorMessage"`
	ResultOptions []string                  `json:"resultOptions"`
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
	result := s.createCheckQuery(r)
	if result.err != nil {
		s.renderAdminPage(w, r, "", result.message)
		return
	}

	http.Redirect(w, r, withFlash("/admin/check-queries", result.message), http.StatusSeeOther)
}

func (s *AdminServer) handleCreateCheckResult(w http.ResponseWriter, r *http.Request) {
	result := s.createCheckResult(r)
	if result.err != nil {
		s.renderAdminPage(w, r, "", result.message)
		return
	}

	http.Redirect(w, r, withFlash("/admin/check-queries", result.message), http.StatusSeeOther)
}

func (s *AdminServer) renderAdminPage(w http.ResponseWriter, r *http.Request, flash, errMsg string) {
	data, err := s.loadAdminData()
	if err != nil {
		http.Error(w, fmt.Sprintf("管理用データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	if flash == "" {
		flash = r.URL.Query().Get("flash")
	}
	if errMsg == "" {
		errMsg = r.URL.Query().Get("error")
	}

	data.FlashMessage = flash
	data.ErrorMessage = errMsg

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "admin.gohtml", data); err != nil {
		http.Error(w, fmt.Sprintf("テンプレートの描画に失敗しました: %v", err), http.StatusInternalServerError)
	}
}

type actionResult struct {
	err     error
	message string
}

func (s *AdminServer) createCheckQuery(r *http.Request) actionResult {
	if err := r.ParseForm(); err != nil {
		return actionResult{
			err:     fmt.Errorf("フォームの解析に失敗しました: %w", err),
			message: "フォームの解析に失敗しました",
		}
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		return actionResult{
			err:     fmt.Errorf("名前は必須です"),
			message: "名前は必須です",
		}
	}

	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	if _, err := s.db.InsertCheckQuery(name, descPtr); err != nil {
		return actionResult{
			err:     fmt.Errorf("チェッククエリの登録に失敗しました: %w", err),
			message: "チェッククエリの登録に失敗しました",
		}
	}

	return actionResult{message: "チェッククエリを登録しました"}
}

func (s *AdminServer) createCheckResult(r *http.Request) actionResult {
	if err := r.ParseForm(); err != nil {
		return actionResult{
			err:     fmt.Errorf("フォームの解析に失敗しました: %w", err),
			message: "フォームの解析に失敗しました",
		}
	}

	repositoryID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("repository_id")))
	if err != nil || repositoryID <= 0 {
		return actionResult{
			err:     fmt.Errorf("リポジトリIDが不正です"),
			message: "リポジトリIDが不正です",
		}
	}

	checkQueryID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("check_query_id")))
	if err != nil || checkQueryID <= 0 {
		return actionResult{
			err:     fmt.Errorf("チェッククエリIDが不正です"),
			message: "チェッククエリIDが不正です",
		}
	}

	result := strings.TrimSpace(r.FormValue("result"))
	if !s.isValidResult(result) {
		return actionResult{
			err:     fmt.Errorf("結果は○, ×, △のいずれかを選択してください"),
			message: "結果は○, ×, △のいずれかを選択してください",
		}
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
		return actionResult{
			err:     fmt.Errorf("Webアプリ判定はTrueかFalseを指定してください"),
			message: "Webアプリ判定はTrueかFalseを指定してください",
		}
	}

	if err := s.db.UpsertMyCheckedRepository(repositoryID, checkQueryID, result, memoPtr); err != nil {
		return actionResult{
			err:     fmt.Errorf("判定結果の保存に失敗しました: %w", err),
			message: "判定結果の保存に失敗しました",
		}
	}

	if err := s.db.UpsertRepositoryWebAppCheck(repositoryID, isWebApp); err != nil {
		return actionResult{
			err:     fmt.Errorf("Webアプリ判定の保存に失敗しました: %w", err),
			message: "Webアプリ判定の保存に失敗しました",
		}
	}

	return actionResult{message: "判定結果を登録しました"}
}

func (s *AdminServer) loadAdminData() (adminPageData, error) {
	queries, err := s.db.GetCheckQueries()
	if err != nil {
		return adminPageData{}, fmt.Errorf("チェッククエリの取得に失敗しました: %w", err)
	}

	repos, err := s.db.GetRepositories(50, 0)
	if err != nil {
		return adminPageData{}, fmt.Errorf("リポジトリ一覧の取得に失敗しました: %w", err)
	}

	checks, err := s.db.ListMyCheckSummaries(50)
	if err != nil {
		return adminPageData{}, fmt.Errorf("判定履歴の取得に失敗しました: %w", err)
	}

	return adminPageData{
		CheckQueries:  queries,
		Repositories:  repos,
		MyChecks:      checks,
		ResultOptions: s.resultOpts,
	}, nil
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
