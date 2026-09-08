package pan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Account struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Avatar string       `json:"avatar"`
	Level  string       `json:"level"`
	Space  AccountSpace `json:"space"`
}

type AccountSpace struct {
	Total     SpaceAmount `json:"total"`
	Used      SpaceAmount `json:"used"`
	Remaining SpaceAmount `json:"remaining"`
}

type SpaceAmount struct {
	Bytes     int64  `json:"bytes"`
	Formatted string `json:"formatted"`
}

type wireSpaceAmount struct {
	Size      json.Number `json:"size"`
	Formatted string      `json:"size_format"`
}

func (value wireSpaceAmount) amount() (SpaceAmount, error) {
	size, err := value.Size.Int64()
	if err != nil {
		return SpaceAmount{}, err
	}
	return SpaceAmount{Bytes: size, Formatted: value.Formatted}, nil
}

func (client *Client) Account(ctx context.Context, accessToken string) (Account, error) {
	response, err := client.request(
		client.http.R().SetContext(ctx).SetAuthToken(accessToken),
		http.MethodGet, apiURL+"/open/user/info",
	)
	if err != nil {
		return Account{}, err
	}
	var result struct {
		apiResponse
		Data struct {
			ID     json.Number `json:"user_id"`
			Name   string      `json:"user_name"`
			Avatar string      `json:"user_face_m"`
			VIP    struct {
				Level string `json:"level_name"`
			} `json:"vip_info"`
			Space struct {
				Total     wireSpaceAmount `json:"all_total"`
				Used      wireSpaceAmount `json:"all_use"`
				Remaining wireSpaceAmount `json:"all_remain"`
			} `json:"rt_space_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return Account{}, fmt.Errorf("decode 115 account: %w", err)
	}
	if err := result.err(); err != nil {
		return Account{}, err
	}
	if result.Data.ID == "" {
		return Account{}, fmt.Errorf("115 account response is missing user_id")
	}
	total, err := result.Data.Space.Total.amount()
	if err != nil {
		return Account{}, fmt.Errorf("decode 115 total space: %w", err)
	}
	used, err := result.Data.Space.Used.amount()
	if err != nil {
		return Account{}, fmt.Errorf("decode 115 used space: %w", err)
	}
	remaining, err := result.Data.Space.Remaining.amount()
	if err != nil {
		return Account{}, fmt.Errorf("decode 115 remaining space: %w", err)
	}
	return Account{
		ID: result.Data.ID.String(), Name: result.Data.Name,
		Avatar: result.Data.Avatar, Level: result.Data.VIP.Level,
		Space: AccountSpace{Total: total, Used: used, Remaining: remaining},
	}, nil
}
