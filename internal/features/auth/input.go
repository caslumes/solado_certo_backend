package auth

type RegisterInput struct {
	Name     string
	Email    string
	Phone    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

type ResetPasswordInput struct {
	Email       string
	Code        string
	NewPassword string
}
