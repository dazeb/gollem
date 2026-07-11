package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

type FsReadFileParams struct {
	Path AbsolutePathBuf `json:"path"`
}

type FsReadFileResponse struct {
	DataBase64       string     `json:"dataBase64"`
	Path             *string    `json:"path,omitempty" jsonschema:"nonnullable=true"`
	Content          *string    `json:"content,omitempty" jsonschema:"nonnullable=true"`
	Encoding         *string    `json:"encoding,omitempty" jsonschema:"nonnullable=true"`
	Size             *int64     `json:"size,omitempty" jsonschema:"nonnullable=true"`
	Mode             *uint32    `json:"mode,omitempty" jsonschema:"nonnullable=true"`
	ModTime          *time.Time `json:"modTime,omitempty" jsonschema:"nonnullable=true"`
	ContentTruncated *bool      `json:"contentTruncated,omitempty" jsonschema:"nonnullable=true"`
}

type FsWriteFileParams struct {
	Path       AbsolutePathBuf `json:"path"`
	DataBase64 string          `json:"dataBase64"`
}

type FsWriteFileResponse struct {
	OK   *bool   `json:"ok,omitempty" jsonschema:"nonnullable=true"`
	Path *string `json:"path,omitempty" jsonschema:"nonnullable=true"`
}

type FsCreateDirectoryParams struct {
	Path      AbsolutePathBuf `json:"path"`
	Recursive *bool           `json:"recursive,omitempty"`
}

type FsCreateDirectoryResponse struct {
	OK   *bool   `json:"ok,omitempty" jsonschema:"nonnullable=true"`
	Path *string `json:"path,omitempty" jsonschema:"nonnullable=true"`
}

type FsGetMetadataParams struct {
	Path AbsolutePathBuf `json:"path"`
}

type FsGetMetadataResponse struct {
	IsDirectory  bool       `json:"isDirectory"`
	IsFile       bool       `json:"isFile"`
	IsSymlink    bool       `json:"isSymlink"`
	CreatedAtMS  int64      `json:"createdAtMs"`
	ModifiedAtMS int64      `json:"modifiedAtMs"`
	Path         *string    `json:"path,omitempty" jsonschema:"nonnullable=true"`
	IsDir        *bool      `json:"isDir,omitempty" jsonschema:"nonnullable=true"`
	Size         *int64     `json:"size,omitempty" jsonschema:"nonnullable=true"`
	Mode         *uint32    `json:"mode,omitempty" jsonschema:"nonnullable=true"`
	ModTime      *time.Time `json:"modTime,omitempty" jsonschema:"nonnullable=true"`
}

type FsReadDirectoryParams struct {
	Path AbsolutePathBuf `json:"path"`
}

type FsReadDirectoryEntry struct {
	FileName      string     `json:"fileName"`
	IsDirectory   bool       `json:"isDirectory"`
	IsFile        bool       `json:"isFile"`
	LegacyPath    *string    `json:"Path,omitempty" jsonschema:"nonnullable=true"`
	LegacyName    *string    `json:"Name,omitempty" jsonschema:"nonnullable=true"`
	LegacyIsDir   *bool      `json:"IsDir,omitempty" jsonschema:"nonnullable=true"`
	LegacySize    *int64     `json:"Size,omitempty" jsonschema:"nonnullable=true"`
	LegacyMode    *uint32    `json:"Mode,omitempty" jsonschema:"nonnullable=true"`
	LegacyModTime *time.Time `json:"ModTime,omitempty" jsonschema:"nonnullable=true"`
}

type FsReadDirectoryResponse struct {
	Entries []FsReadDirectoryEntry `json:"entries" jsonschema:"nonnullable=true"`
}

type FsRemoveParams struct {
	Path      AbsolutePathBuf `json:"path"`
	Recursive *bool           `json:"recursive,omitempty"`
	Force     *bool           `json:"force,omitempty"`
}

type FsRemoveResponse struct {
	OK   *bool   `json:"ok,omitempty" jsonschema:"nonnullable=true"`
	Path *string `json:"path,omitempty" jsonschema:"nonnullable=true"`
}

type FsCopyParams struct {
	SourcePath      AbsolutePathBuf `json:"sourcePath"`
	DestinationPath AbsolutePathBuf `json:"destinationPath"`
	Recursive       *bool           `json:"recursive,omitempty" jsonschema:"nonnullable=true"`
}

type FsCopyResponse struct {
	OK          *bool   `json:"ok,omitempty" jsonschema:"nonnullable=true"`
	Source      *string `json:"source,omitempty" jsonschema:"nonnullable=true"`
	Destination *string `json:"destination,omitempty" jsonschema:"nonnullable=true"`
}

type FsWatchParams struct {
	WatchID            string          `json:"watchId"`
	Path               AbsolutePathBuf `json:"path"`
	PollIntervalMillis *int64          `json:"pollIntervalMillis,omitempty" jsonschema:"nonnullable=true"`
}

type FsWatchResponse struct {
	Path AbsolutePathBuf `json:"path"`
}

type FsUnwatchParams struct {
	WatchID string `json:"watchId"`
}

type FsUnwatchResponse struct{}

type FsChangedNotification struct {
	WatchID      string            `json:"watchId"`
	ChangedPaths []AbsolutePathBuf `json:"changedPaths" jsonschema:"nonnullable=true"`
}

// FileChangedNotification is Gollem's mutation-level extension on fs/changed.
// The public watch notification remains FsChangedNotification.
type FileChangedNotification struct {
	Path        *string   `json:"path,omitempty" jsonschema:"nonnullable=true"`
	Destination *string   `json:"destination,omitempty" jsonschema:"nonnullable=true"`
	Operation   string    `json:"operation"`
	At          time.Time `json:"at"`
}

func (r FsReadDirectoryResponse) MarshalJSON() ([]byte, error) {
	type wire FsReadDirectoryResponse
	if r.Entries == nil {
		r.Entries = []FsReadDirectoryEntry{}
	}
	return json.Marshal(wire(r))
}

func (r *FsReadDirectoryResponse) UnmarshalJSON(data []byte) error {
	type wire FsReadDirectoryResponse
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Entries == nil {
		return errors.New("filesystem directory entries must be a non-null array")
	}
	*r = FsReadDirectoryResponse(decoded)
	return nil
}

func (n FsChangedNotification) MarshalJSON() ([]byte, error) {
	type wire FsChangedNotification
	if n.ChangedPaths == nil {
		n.ChangedPaths = []AbsolutePathBuf{}
	}
	return json.Marshal(wire(n))
}

func (n *FsChangedNotification) UnmarshalJSON(data []byte) error {
	type wire FsChangedNotification
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.ChangedPaths == nil {
		return errors.New("filesystem changed paths must be a non-null array")
	}
	*n = FsChangedNotification(decoded)
	return nil
}
