package mini

type MetaEnvelope struct {
    Path string
    Data Meta
}

type Meta struct {
    Client      string
    Environment string
    Id          string
    Name        string
    Kind        string
    Description string
    Version     string
    Config      string
}
