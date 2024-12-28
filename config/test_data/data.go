package test_data

type Person struct {
	Name       string   `json:"name" toml:"name" yaml:"name"`
	Age        int      `json:"age" toml:"age" yaml:"age"`
	IsStudent  bool     `json:"is_student" toml:"is_student" yaml:"is_student"`
	Scores     []int    `json:"scores" toml:"scores" yaml:"scores"`
	EmptyField any      `json:"empty_field" toml:"empty_field" yaml:"empty_field,omitempty"`
	Address    Address  `json:"address" toml:"address" yaml:"address"`
	Hobbies    []string `json:"hobbies" toml:"hobbies" yaml:"hobbies"`
}

type Address struct {
	Street  string `json:"street" toml:"street" yaml:"street"`
	City    string `json:"city" toml:"city" yaml:"city"`
	State   string `json:"state" toml:"state" yaml:"state"`
	ZipCode string `json:"zip_code" toml:"zip_code" yaml:"zip_code"`
}
