package app

import "errors"

var ErrInvalidInput = errors.New("invalid input")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")
