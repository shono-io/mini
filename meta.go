package mini

type MetaEnvelope struct {
	Path string `json:"path"`
	Data Meta   `json:"data"`
}

type Meta struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Config      string `json:"config"`
}
