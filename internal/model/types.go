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
	Text    string  `json:"text,omitempty"`
	Matches []Match `json:"matches,omitempty"`
}

type SymbolItem struct {
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Container string `json:"container,omitempty"`
	Lang      string `json:"lang,omitempty"`
	Signature string `json:"signature,omitempty"`
	Path      string `json:"path"`
	Range     Range  `json:"range"`
}

type CommentItem struct {
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Lang  string `json:"lang,omitempty"`
	Path  string `json:"path"`
	Range Range  `json:"range"`
}
