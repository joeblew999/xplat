package taskui

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
)

// TaskInfo represents a task for the UI.
type TaskInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Summary     string `json:"summary,omitempty"`
}

// handleIndex renders the main page with task list.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.listTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tasks":    tasks,
		"Taskfile": s.config.Taskfile,
		"WorkDir":  s.config.WorkDir,
	}

	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleTask renders the task execution page.
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	tasks, err := s.listTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the task
	var task *TaskInfo
	for i := range tasks {
		if tasks[i].Name == name {
			task = &tasks[i]
			break
		}
	}

	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	data := map[string]interface{}{
		"Task":     task,
		"Tasks":    tasks,
		"Taskfile": s.config.Taskfile,
		"WorkDir":  s.config.WorkDir,
	}

	if err := s.templates.ExecuteTemplate(w, "task.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAPITasks returns the task list as JSON.
func (s *Server) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.listTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// listTasks returns the list of tasks from the Taskfile.
func (s *Server) listTasks() ([]TaskInfo, error) {
	tf, err := parseTaskfile(s.config.Taskfile, s.config.WorkDir)
	if err != nil {
		return nil, err
	}

	var tasks []TaskInfo
	for name, task := range tf.Tasks {
		// Skip internal tasks (those starting with _)
		if len(name) > 0 && name[0] == '_' {
			continue
		}
		// Skip tasks marked as internal
		if task.Internal {
			continue
		}

		tasks = append(tasks, TaskInfo{
			Name:        name,
			Description: task.Desc,
			Summary:     task.Summary,
		})
	}

	// Sort by name
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Name < tasks[j].Name
	})

	return tasks, nil
}
