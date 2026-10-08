package download

import "encoding/json"

// Config is shared by subscription selection and offline recovery.
type Config struct {
	AutoSwitch bool `json:"auto_switch"`
}

func DefaultConfig() Config {
	return Config{AutoSwitch: true}
}

func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	value := plain(DefaultConfig())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = Config(value)
	return nil
}
