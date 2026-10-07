package enum

type ProfileStatus string

const (
	ProfileStatusActive   ProfileStatus = "active"
	ProfileStatusPending  ProfileStatus = "pending"
	ProfileStatusInactive ProfileStatus = "inactive"
	ProfileStatusBlocked  ProfileStatus = "blocked"
)
