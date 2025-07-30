package services

import "fmt"

type Error interface {
	GetMessage() string
	GetStatusCode() int
}

type ErrorDetails struct {
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

func (e *ErrorDetails) GetMessage() string {
	return e.Message
}

func (e *ErrorDetails) GetStatusCode() int {
	return e.StatusCode
}

func Errorf(statusCode int, message string, params ...any) Error {
	return &ErrorDetails{
		StatusCode: statusCode,
		Message:    fmt.Sprintf(message, params...),
	}
}
