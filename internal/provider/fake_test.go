package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"

	"google.golang.org/api/script/v1"
)

// fakeAPI is an in-memory fake of the Apps Script and Drive APIs, implementing
// only what the provider uses. It is used as an http.RoundTripper.
type fakeAPI struct {
	mu       sync.Mutex
	mux      *http.ServeMux
	projects map[string]*fakeProject
	nextID   int
}

type fakeProject struct {
	script.Project
	files       []*script.File
	trashed     bool
	versions    int64
	deployments map[string]*script.Deployment
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{mux: http.NewServeMux(), projects: map[string]*fakeProject{}}

	f.mux.HandleFunc("POST /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		var req script.CreateProjectRequest
		if !decode(w, r, &req) {
			return
		}
		p := &fakeProject{
			Project:     script.Project{ScriptId: f.id("script"), Title: req.Title, ParentId: req.ParentId},
			deployments: map[string]*script.Deployment{},
		}
		f.projects[p.ScriptId] = p
		reply(w, p.Project)
	})
	f.mux.HandleFunc("GET /v1/projects/{scriptId}", func(w http.ResponseWriter, r *http.Request) {
		if p := f.project(w, r); p != nil {
			reply(w, p.Project)
		}
	})
	f.mux.HandleFunc("GET /v1/projects/{scriptId}/content", func(w http.ResponseWriter, r *http.Request) {
		if p := f.project(w, r); p != nil {
			reply(w, script.Content{ScriptId: p.ScriptId, Files: p.files})
		}
	})
	f.mux.HandleFunc("PUT /v1/projects/{scriptId}/content", func(w http.ResponseWriter, r *http.Request) {
		var c script.Content
		if p := f.project(w, r); p != nil && decode(w, r, &c) {
			p.files = c.Files
			reply(w, c)
		}
	})
	f.mux.HandleFunc("POST /v1/projects/{scriptId}/versions", func(w http.ResponseWriter, r *http.Request) {
		var v script.Version
		if p := f.project(w, r); p != nil && decode(w, r, &v) {
			p.versions++
			v.ScriptId, v.VersionNumber = p.ScriptId, p.versions
			reply(w, v)
		}
	})
	f.mux.HandleFunc("POST /v1/projects/{scriptId}/deployments", func(w http.ResponseWriter, r *http.Request) {
		var c script.DeploymentConfig
		if p := f.project(w, r); p != nil && decode(w, r, &c) && p.validVersion(w, c.VersionNumber) {
			d := &script.Deployment{DeploymentId: f.id("deployment"), DeploymentConfig: &c}
			p.deployments[d.DeploymentId] = d
			reply(w, p.withEntryPoints(d))
		}
	})
	f.mux.HandleFunc("GET /v1/projects/{scriptId}/deployments/{deploymentId}", func(w http.ResponseWriter, r *http.Request) {
		if p, d := f.deployment(w, r); d != nil {
			reply(w, p.withEntryPoints(d))
		}
	})
	f.mux.HandleFunc("PUT /v1/projects/{scriptId}/deployments/{deploymentId}", func(w http.ResponseWriter, r *http.Request) {
		var req script.UpdateDeploymentRequest
		if p, d := f.deployment(w, r); d != nil && decode(w, r, &req) && p.validVersion(w, req.DeploymentConfig.VersionNumber) {
			d.DeploymentConfig = req.DeploymentConfig
			reply(w, p.withEntryPoints(d))
		}
	})
	f.mux.HandleFunc("DELETE /v1/projects/{scriptId}/deployments/{deploymentId}", func(w http.ResponseWriter, r *http.Request) {
		if p, d := f.deployment(w, r); d != nil {
			delete(p.deployments, d.DeploymentId)
			reply(w, struct{}{})
		}
	})

	f.mux.HandleFunc("GET /drive/v3/files/{fileId}", func(w http.ResponseWriter, r *http.Request) {
		if p := f.file(w, r); p != nil {
			reply(w, map[string]any{"id": p.ScriptId, "name": p.Title, "trashed": p.trashed})
		}
	})
	f.mux.HandleFunc("PATCH /drive/v3/files/{fileId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name    string `json:"name"`
			Trashed bool   `json:"trashed"`
		}
		if p := f.file(w, r); p != nil && decode(w, r, &req) {
			if req.Name != "" {
				p.Title = req.Name
			}
			p.trashed = p.trashed || req.Trashed
			reply(w, map[string]any{"id": p.ScriptId, "name": p.Title, "trashed": p.trashed})
		}
	})

	return f
}

func (f *fakeAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, r)
	return w.Result(), nil
}

func (f *fakeAPI) id(prefix string) string {
	f.nextID++
	return prefix + "-" + strconv.Itoa(f.nextID)
}

func (f *fakeAPI) project(w http.ResponseWriter, r *http.Request) *fakeProject {
	p, ok := f.projects[r.PathValue("scriptId")]
	if !ok {
		replyError(w, http.StatusNotFound, "project not found")
		return nil
	}
	return p
}

func (f *fakeAPI) deployment(w http.ResponseWriter, r *http.Request) (*fakeProject, *script.Deployment) {
	p := f.project(w, r)
	if p == nil {
		return nil, nil
	}
	d, ok := p.deployments[r.PathValue("deploymentId")]
	if !ok {
		replyError(w, http.StatusNotFound, "deployment not found")
		return nil, nil
	}
	return p, d
}

// file returns a standalone project by Drive file ID. Container-bound projects
// are not Drive files.
func (f *fakeAPI) file(w http.ResponseWriter, r *http.Request) *fakeProject {
	p, ok := f.projects[r.PathValue("fileId")]
	if !ok || p.ParentId != "" {
		replyError(w, http.StatusNotFound, "file not found")
		return nil
	}
	return p
}

func (p *fakeProject) validVersion(w http.ResponseWriter, n int64) bool {
	if n < 1 || n > p.versions {
		replyError(w, http.StatusBadRequest, fmt.Sprintf("version %d not found", n))
		return false
	}
	return true
}

// withEntryPoints adds a web app entry point if the manifest configures one.
func (p *fakeProject) withEntryPoints(d *script.Deployment) *script.Deployment {
	out := *d
	for _, f := range p.files {
		var m struct {
			WebApp any `json:"webapp"`
		}
		if f.Type == "JSON" && json.Unmarshal([]byte(f.Source), &m) == nil && m.WebApp != nil {
			out.EntryPoints = []*script.EntryPoint{{
				EntryPointType: "WEB_APP",
				WebApp:         &script.GoogleAppsScriptTypeWebAppEntryPoint{Url: "https://script.google.com/macros/s/" + d.DeploymentId + "/exec"},
			}}
		}
	}
	return &out
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func replyError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}
