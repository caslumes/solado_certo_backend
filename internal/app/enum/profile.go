package enum

type ProfileStatus string

const (
	ProfileStatusActive   ProfileStatus = "ativo"
	ProfileStatusPending  ProfileStatus = "pendente"
	ProfileStatusInactive ProfileStatus = "inativo"
	ProfileStatusBlocked  ProfileStatus = "bloqueado"
)
