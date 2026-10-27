package main

// ListProjectTopics reads current registered sessions.
func (a *App) ListProjectTopics(req ProjectTopicPageRequest) (ProjectTopicPage, error) {
	return a.unifiedProjectTopics(req)
}
