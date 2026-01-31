package model

type Range struct {
	SL int `json:"sl"`
	SC int `json:"sc"`
	EL int `json:"el"`
	EC int `json:"ec"`
}

type Match struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Text string `json:"text"`
}

type ResultItem struct {
	Kind    string  `json:"kind,omitempty"`
	Path    string  `json:"path"`
	Range   Range   `json:"range"`
	Title   string  `json:"title,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Matches []Match `json:"matches,omitempty"`
}

