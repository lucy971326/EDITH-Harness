package web

import (
	"errors"
	"net/http"
	"net/url"
)

func (s *service) handlePickProject(writer http.ResponseWriter, request *http.Request) {
	root, err := s.picker.Pick(request.Context())
	if errors.Is(err, errDirectoryPickCancelled) {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	project, err := s.projects.Create(root)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?project="+url.QueryEscape(project.ID)+"&draft=1", http.StatusSeeOther)
}
