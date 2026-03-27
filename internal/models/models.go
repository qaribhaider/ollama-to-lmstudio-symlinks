package models

// OllamaManifest represents the structure of an Ollama model manifest
type OllamaManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"layers"`
}

// ModelInfo holds information about a discovered model
type ModelInfo struct {
	Name            string
	MainModelBlob   string
	AdditionalBlobs map[string]string // blob_hash -> suggested_filename
}

// LMStudioModel holds information about a model found in LM Studio
type LMStudioModel struct {
	Name string
	Path string
}
