package api

func (h *Handler) GetRoutes() []Route {
	return []Route{
		{
			"GetNodes",
			"GET",
			"/node",
			h.GetNodes,
		},
	}
}
