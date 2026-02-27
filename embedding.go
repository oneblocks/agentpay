package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type MoonshotEmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type MoonshotEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func GetEmbedding(text string) ([]float64, error) {

	reqBody := MoonshotEmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: text,
	}

	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(
		"POST",
		"https://api.moonshot.cn/v1/embeddings",
		bytes.NewBuffer(jsonData),
	)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("MOONSHOT_API_KEY"))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var result MoonshotEmbeddingResponse
	json.Unmarshal(body, &result)

	return result.Data[0].Embedding, nil
}
