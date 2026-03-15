package team

// TeammateSpawnedEvent is published when a new teammate is created.
type TeammateSpawnedEvent struct {
	TeamName     string
	TeammateName string
	Task         string
}

// TeammateIdleEvent is published when a teammate is waiting for more team tasks.
type TeammateIdleEvent struct {
	TeamName     string
	TeammateName string
}

// TeammateTerminatedEvent is published when a teammate shuts down.
type TeammateTerminatedEvent struct {
	TeamName     string
	TeammateName string
	Reason       string
}
