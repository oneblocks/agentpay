package ai

import "math"

type VectorItem struct {
    ServiceName string
    Vector      []float64
}

var VectorDB []VectorItem

func CosineSimilarity(a, b []float64) float64 {

    var dot, normA, normB float64

    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }

    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func SearchBestMatch(queryVector []float64) string {

    bestScore := -1.0
    bestService := ""

    for _, item := range VectorDB {
        score := CosineSimilarity(queryVector, item.Vector)
        if score > bestScore {
            bestScore = score
            bestService = item.ServiceName
        }
    }

    return bestService
}
