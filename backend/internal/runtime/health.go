package runtime

// HealthInfo holds runtime-agnostic health check information.
type HealthInfo struct {
	HasHealthCheck bool
	Status         string // "healthy", "unhealthy", "starting", "none"
	FailingStreak  int
	LastOutput     string
}
