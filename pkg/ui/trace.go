package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

func buildRunTraceView(runID string, artifact *traceutil.Artifact) RunTraceView {
	view := RunTraceView{
		URL:        "/runs/" + runID + "/trace",
		InspectURL: "/runs/" + runID + "/trace/inspect",
	}
	if artifact == nil {
		return view
	}
	view.Available = true
	view.SchemaVersion = artifact.SchemaVersion
	view.Status = artifact.Summary.Status
	view.Steps = artifact.Summary.Steps
	view.Events = len(artifact.Events)
	view.Snapshots = len(artifact.Snapshots)
	view.Requests = artifact.Summary.Requests
	view.ToolCalls = artifact.Summary.ToolCalls
	return view
}

func (r *RunRecord) traceArtifactSnapshot() (*traceutil.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.traceArtifact == nil {
		return nil, nil
	}
	return cloneTraceArtifact(r.traceArtifact)
}

func cloneTraceArtifact(src *traceutil.Artifact) (*traceutil.Artifact, error) {
	if src == nil {
		return nil, nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst traceutil.Artifact
	if err := json.Unmarshal(data, &dst); err != nil {
		return nil, err
	}
	return &dst, nil
}

func (s *Server) handleRunTrace(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	artifact, err := run.traceArtifactSnapshot()
	if err != nil {
		http.Error(w, fmt.Sprintf("clone trace artifact: %v", err), http.StatusInternalServerError)
		return
	}
	if artifact == nil {
		http.Error(w, "trace artifact is not available yet", http.StatusNotFound)
		return
	}

	var buf bytes.Buffer
	if err := traceutil.Write(&buf, artifact); err != nil {
		http.Error(w, fmt.Sprintf("encode trace artifact: %v", err), http.StatusInternalServerError)
		return
	}
	filename := run.ID() + ".trace.json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleRunTraceInspect(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	artifact, err := run.traceArtifactSnapshot()
	if err != nil {
		http.Error(w, fmt.Sprintf("clone trace artifact: %v", err), http.StatusInternalServerError)
		return
	}
	if artifact == nil {
		http.Error(w, "trace artifact is not available yet", http.StatusNotFound)
		return
	}

	var buf bytes.Buffer
	if err := traceutil.Inspect(&buf, artifact, traceutil.InspectOptions{}); err != nil {
		http.Error(w, fmt.Sprintf("inspect trace artifact: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
