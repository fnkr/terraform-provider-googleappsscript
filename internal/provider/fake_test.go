package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/script/v1"
)

// fakeAPI is an in-memory fake of the Apps Script and Drive APIs, implementing
// only what the provider uses. It is used as an http.RoundTripper.
type fakeAPI struct {
	mu       sync.Mutex
	mux      *http.ServeMux
	projects map[string]*fakeProject
	files    map[string]*drive.File
	nextID   int
}

type fakeProject struct {
	script.Project
	files       []*script.File
	versions    int64
	deployments map[string]*script.Deployment
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{mux: http.NewServeMux(), projects: map[string]*fakeProject{}, files: map[string]*drive.File{}}

	f.mux.HandleFunc("POST /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		var req script.CreateProjectRequest
		if !decode(w, r, &req) {
			return
		}
		p := &fakeProject{
			Project:     script.Project{ScriptId: f.id("script"), Title: req.Title, ParentId: req.ParentId},
			deployments: map[string]*script.Deployment{},
		}
		if req.ParentId != "" {
			if _, ok := f.files[req.ParentId]; !ok {
				replyError(w, http.StatusNotFound, "parent not found")
				return
			}
		} else {
			// Standalone projects are Drive files; bound projects are not.
			f.files[p.ScriptId] = &drive.File{Id: p.ScriptId, Name: req.Title, MimeType: googleAppsMimeTypePrefix + "script"}
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

	f.mux.HandleFunc("POST /drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		var file drive.File
		if decode(w, r, &file) {
			file.Id = f.id("file")
			file.WebViewLink = "https://docs.google.com/fake/" + file.Id
			f.files[file.Id] = &file
			reply(w, file)
		}
	})
	f.mux.HandleFunc("GET /drive/v3/files/{fileId}", func(w http.ResponseWriter, r *http.Request) {
		if file := f.file(w, r); file != nil {
			reply(w, file)
		}
	})
	f.mux.HandleFunc("PATCH /drive/v3/files/{fileId}", func(w http.ResponseWriter, r *http.Request) {
		var req drive.File
		// Like the real API, Drive can rename bound projects but not read or trash them.
		if p, ok := f.projects[r.PathValue("fileId")]; ok && p.ParentId != "" {
			if !decode(w, r, &req) {
				return
			}
			if req.Trashed {
				replyError(w, http.StatusBadRequest, "bad request")
				return
			}
			p.Title = req.Name
			reply(w, drive.File{Id: p.ScriptId, Name: p.Title})
			return
		}
		if file := f.file(w, r); file != nil && decode(w, r, &req) {
			if req.Name != "" {
				file.Name = req.Name
				if p, ok := f.projects[file.Id]; ok {
					p.Title = req.Name
				}
			}
			file.Trashed = file.Trashed || req.Trashed
			reply(w, file)
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

func (f *fakeAPI) file(w http.ResponseWriter, r *http.Request) *drive.File {
	file, ok := f.files[r.PathValue("fileId")]
	if !ok {
		replyError(w, http.StatusNotFound, "file not found")
		return nil
	}
	return file
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
