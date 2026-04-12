package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jtb75/silkstrand/backoffice/internal/crypto"
	"github.com/jtb75/silkstrand/backoffice/internal/dcclient"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// dcConnFromRecord decrypts the API key from a DataCenter record and returns a DCConn.
func dcConnFromRecord(dc *model.DataCenter, encKey []byte) (*dcclient.DCConn, error) {
	apiKey, err := crypto.Decrypt(dc.APIKeyEncrypted, encKey)
	if err != nil {
		return nil, err
	}
	return &dcclient.DCConn{
		APIURL: dc.APIURL,
		APIKey: string(apiKey),
	}, nil
}
