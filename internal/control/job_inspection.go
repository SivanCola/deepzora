package control

import "deepzora/internal/jobs"

func (c *Controller) InspectBackgroundJob(id string) (jobs.Result, bool) {
	parent := c.parentSessionID()
	if c.jobs == nil || parent == "" {
		return jobs.Result{}, false
	}
	return c.jobs.InspectForSession(parent, id)
}
