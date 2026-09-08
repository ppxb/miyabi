package pan

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func SHA1(body []byte) string {
	sum := sha1.Sum(body)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

type uploadResult struct {
	Status    int             `json:"status"`
	SignKey   string          `json:"sign_key"`
	SignCheck string          `json:"sign_check"`
	Bucket    string          `json:"bucket"`
	Object    string          `json:"object"`
	Callback  json.RawMessage `json:"callback"`
}

// UploadMetadata uploads one small sidecar. Callers decide its destination and
// whether a same-name file already exists; this endpoint is not an overwrite.
func (client *Client) UploadMetadata(ctx context.Context, accessToken, directoryID, name string, body []byte) error {
	form := map[string]string{
		"file_name": name, "file_size": strconv.Itoa(len(body)), "target": "U_1_" + directoryID,
		"fileid": SHA1(body), "preid": SHA1(body[:min(len(body), 128*1024)]),
	}
	init, err := client.initUpload(ctx, accessToken, form)
	if err != nil {
		return err
	}
	if init.Status == 6 || init.Status == 7 || init.Status == 8 {
		startText, endText, ok := strings.Cut(init.SignCheck, "-")
		start, startErr := strconv.Atoi(startText)
		end, endErr := strconv.Atoi(endText)
		if !ok || startErr != nil || endErr != nil || start < 0 || end < start || end >= len(body) {
			return fmt.Errorf("115 returned invalid upload challenge range %q", init.SignCheck)
		}
		form["sign_key"], form["sign_val"] = init.SignKey, SHA1(body[start:end+1])
		init, err = client.initUpload(ctx, accessToken, form)
		if err != nil {
			return err
		}
	}
	if init.Status == 2 {
		return nil
	}
	if init.Bucket == "" || init.Object == "" {
		return fmt.Errorf("115 returned unsupported upload status %d", init.Status)
	}
	var callback struct {
		Callback  string `json:"callback"`
		Variables string `json:"callback_var"`
	}
	if err := json.Unmarshal(init.Callback, &callback); err != nil {
		return fmt.Errorf("decode 115 upload callback: %w", err)
	}
	if callback.Callback == "" {
		return fmt.Errorf("115 upload callback is empty")
	}
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken),
		http.MethodGet, apiURL+"/open/upload/get_token")
	if err != nil {
		return err
	}
	var token struct {
		apiResponse
		Data struct {
			Endpoint string `json:"endpoint"`
			KeyID    string `json:"AccessKeyId"`
			Secret   string `json:"AccessKeySecret"`
			Token    string `json:"SecurityToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &token); err != nil {
		return fmt.Errorf("decode 115 upload token: %w", err)
	}
	if err := token.err(); err != nil {
		return err
	}
	storage, err := oss.New(token.Data.Endpoint, token.Data.KeyID, token.Data.Secret,
		oss.SecurityToken(token.Data.Token), oss.HTTPClient(client.http.GetClient()))
	if err != nil {
		return fmt.Errorf("initialize 115 object upload: %w", err)
	}
	bucket, err := storage.Bucket(init.Bucket)
	if err != nil {
		return err
	}
	var result []byte
	if err := bucket.PutObject(init.Object, bytes.NewReader(body), oss.WithContext(ctx),
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(callback.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(callback.Variables))),
		oss.CallbackResult(&result)); err != nil {
		return fmt.Errorf("upload 115 metadata object: %w", err)
	}
	var saved apiResponse
	if err := json.Unmarshal(result, &saved); err != nil {
		return fmt.Errorf("decode 115 upload completion: %w", err)
	}
	return saved.err()
}

func (client *Client) initUpload(ctx context.Context, accessToken string, form map[string]string) (uploadResult, error) {
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetFormData(form),
		http.MethodPost, apiURL+"/open/upload/init")
	if err != nil {
		return uploadResult{}, err
	}
	var result struct {
		apiResponse
		Data uploadResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return uploadResult{}, fmt.Errorf("decode 115 upload initialization: %w", err)
	}
	return result.Data, result.err()
}
