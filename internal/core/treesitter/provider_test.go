//go:build treesitter && cgo

package treesitter

import "testing"

func TestExtractGoSymbolsAndComments(t *testing.T) {
	src := []byte(`package main
// Hello doc
func Hello() { println("x") }
`)
	p := NewProvider()
	syms, comms, err := p.Extract("a.go", src)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(syms) == 0 {
		t.Fatalf("no symbols")
	}
	if len(comms) == 0 {
		t.Fatalf("no comments")
	}
}

func TestExtract_MultiLanguages(t *testing.T) {
	cases := []struct {
		path string
		src  string
	}{
		{
			path: "a.java",
			src: `// hi
public class Foo {
  public Foo() {}
  public void bar() {}
}
`,
		},
		{
			path: "a.py",
			src: `# hi
class Foo:
  def bar(self):
    pass
`,
		},
		{
			path: "a.js",
			src: `// hi
class Foo { bar() {} }
function baz() {}
`,
		},
		{
			path: "a.ts",
			src: `// hi
export interface Foo { a: string }
export class Bar { baz(): void {} }
`,
		},
		{
			path: "a.tsx",
			src: `// hi
export class Foo { bar(): void {} }
`,
		},
		{
			path: "a.c",
			src: `// hi
int add(int a, int b) { return a + b; }
`,
		},
		{
			path: "a.cpp",
			src: `// hi
namespace N {
class Foo { public: void bar() {} };
}
`,
		},
		{
			path: "a.php",
			src: `<?php
// hi
namespace N;
class Foo { function bar() {} }
function baz() {}
`,
		},
		{
			path: "a.cs",
			src: `// hi
namespace N {
  class Foo {
    public Foo() {}
    public void Bar() {}
  }
}
`,
		},
		{
			path: "a.json",
			src: `{
  // hi
  "name": "x",
  "obj": { "inner": 1 }
}
`,
		},
		{
			path: "a.sh",
			src: `# hi
foo() { echo hi; }
`,
		},
	}

	p := NewProvider()
	for _, c := range cases {
		syms, _, err := p.Extract(c.path, []byte(c.src))
		if err != nil {
			t.Fatalf("%s extract: %v", c.path, err)
		}
		if len(syms) == 0 {
			t.Fatalf("%s: no symbols", c.path)
		}
	}
}
