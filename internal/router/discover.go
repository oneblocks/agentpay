package router

import (
    "agentpay/internal/ai"
)

func RegisterServiceWithEmbedding(s Service) {

    description := s.Name + " pricing model " + s.Pricing.Mode

    vector, err := ai.GetEmbedding(description)
    if err != nil {
        panic(err)
    }

    ai.VectorDB = append(ai.VectorDB, ai.VectorItem{
        ServiceName: s.Name,
        Vector:      vector,
    })
}
