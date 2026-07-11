package protocol

type ThreadLoadedListParams struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint32 `json:"limit,omitempty"`
}

type ThreadLoadedListResponse struct {
	Data       []string `json:"data" jsonschema:"nonnullable=true"`
	NextCursor *string  `json:"nextCursor" jsonschema:"optional=true"`
}

type ThreadArchiveParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadArchiveParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

type ThreadArchiveResponse struct {
	Thread *ThreadRecord `json:"thread,omitempty"`
}

type ThreadUnarchiveParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadUnarchiveParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

type ThreadUnarchiveResponse struct {
	Thread ThreadRecord `json:"thread"`
}

type ThreadDeleteParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadDeleteParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

type ThreadDeleteResponse struct {
	Thread *ThreadRecord `json:"thread,omitempty"`
}

type ThreadUnsubscribeParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadUnsubscribeParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

type ThreadUnsubscribeStatus string

const (
	ThreadUnsubscribeNotLoaded     ThreadUnsubscribeStatus = "notLoaded"
	ThreadUnsubscribeNotSubscribed ThreadUnsubscribeStatus = "notSubscribed"
	ThreadUnsubscribeUnsubscribed  ThreadUnsubscribeStatus = "unsubscribed"
)

type ThreadUnsubscribeResponse struct {
	Status ThreadUnsubscribeStatus `json:"status"`
}

type ThreadSetNameParams struct {
	ThreadID string `json:"threadId"`
	Name     string `json:"name"`
	ID       string `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
}

func (p ThreadSetNameParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

func (p ThreadSetNameParams) EffectiveName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Title
}

type ThreadSetNameResponse struct {
	ThreadID string        `json:"threadId,omitempty"`
	Name     string        `json:"name,omitempty"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
}

func firstThreadControlID(primary, legacy string) string {
	if primary != "" {
		return primary
	}
	return legacy
}
