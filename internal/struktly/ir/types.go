package ir

type Service struct {
	Name     string
	Handlers []Handler
}

type Handler struct {
	Name        string
	Method      string
	Path        string
	Description string
}
