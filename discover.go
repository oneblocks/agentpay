package main

func RegisterServiceWithEmbedding(s Service) {

    description := s.Name + " pricing model " + s.Pricing.Mode

    vector, err := GetEmbedding(description)
    if err != nil {
        panic(err)
    }

    VectorDB = append(VectorDB, VectorItem{
        ServiceName: s.Name,
        Vector:      vector,
    })
}
