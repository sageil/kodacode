package search

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"sync"
)

type pythonSymbolEnricher struct{}

const pythonMetadataLineWindow = 3

func (pythonSymbolEnricher) Name() string { return "python-ast" }

func (pythonSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "python" && pythonRuntimePath() != ""
}

func (pythonSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	metadata, err := loadPythonMetadata(path)
	if err != nil || metadata.empty() {
		return symbols
	}

	out := cloneSymbols(symbols)
	for i := range out {
		if meta, ok := metadata.match(out[i]); ok {
			out[i] = mergeSymbolMetadata(out[i], Symbol{
				FilePath:  out[i].FilePath,
				Name:      out[i].Name,
				Kind:      out[i].Kind,
				Language:  "python",
				Line:      out[i].Line,
				Parent:    meta.Parent,
				Signature: meta.Signature,
				Doc:       meta.Doc,
				Tokens:    out[i].Tokens,
			})
		}
	}
	return out
}

type pythonSymbolMetadata struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Parent    string `json:"parent,omitempty"`
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`
}

type pythonMetadataIndex struct {
	exact     map[string]pythonSymbolMetadata
	byNameKey map[string][]pythonSymbolMetadata
}

func pythonSymbolKey(sym Symbol) string {
	return strconv.Itoa(sym.Line) + ":" + sym.Name + ":" + sym.Kind
}

func pythonMetadataNameKey(name, kind string) string {
	return name + ":" + kind
}

func (idx pythonMetadataIndex) empty() bool {
	return len(idx.exact) == 0 && len(idx.byNameKey) == 0
}

func (idx pythonMetadataIndex) match(sym Symbol) (pythonSymbolMetadata, bool) {
	if meta, ok := idx.exact[pythonSymbolKey(sym)]; ok {
		return meta, true
	}

	candidates := idx.byNameKey[pythonMetadataNameKey(sym.Name, sym.Kind)]
	if len(candidates) == 0 {
		return pythonSymbolMetadata{}, false
	}

	best := -1
	bestDistance := pythonMetadataLineWindow + 1
	for i, candidate := range candidates {
		distance := intAbs(candidate.Line - sym.Line)
		if distance > pythonMetadataLineWindow {
			continue
		}
		if distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	if best >= 0 {
		return candidates[best], true
	}
	return pythonSymbolMetadata{}, false
}

func loadPythonMetadata(path string) (pythonMetadataIndex, error) {
	python := pythonRuntimePath()
	if python == "" {
		return pythonMetadataIndex{}, nil
	}

	cmd := exec.Command(python, "-c", pythonEnricherScript, path)
	out, err := cmd.Output()
	if err != nil {
		return pythonMetadataIndex{}, err
	}

	var entries []pythonSymbolMetadata
	if err := json.Unmarshal(out, &entries); err != nil {
		return pythonMetadataIndex{}, err
	}

	index := pythonMetadataIndex{
		exact:     make(map[string]pythonSymbolMetadata, len(entries)),
		byNameKey: make(map[string][]pythonSymbolMetadata, len(entries)),
	}
	for _, entry := range entries {
		key := pythonMetadataNameKey(entry.Name, entry.Kind)
		index.exact[strconv.Itoa(entry.Line)+":"+entry.Name+":"+entry.Kind] = entry
		index.byNameKey[key] = append(index.byNameKey[key], entry)
	}
	return index, nil
}

var (
	pythonPathOnce sync.Once
	cachedPython   string
)

func pythonRuntimePath() string {
	pythonPathOnce.Do(func() {
		for _, name := range []string{"python3", "python"} {
			if path, err := exec.LookPath(name); err == nil {
				cachedPython = path
				return
			}
		}
	})
	return cachedPython
}

func intAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

const pythonEnricherScript = `
import ast
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    source = fh.read()

tree = ast.parse(source, path)

def render(expr):
    if expr is None:
        return ""
    unparse = getattr(ast, "unparse", None)
    if unparse is None:
        return ""
    try:
        return unparse(expr)
    except Exception:
        return ""

def render_args(args):
    parts = []
    posonly = list(getattr(args, "posonlyargs", []))
    positional = posonly + list(args.args)
    defaults = [None] * (len(positional) - len(args.defaults)) + list(args.defaults)

    for idx, arg in enumerate(positional):
        text = arg.arg
        ann = render(arg.annotation)
        if ann:
            text += ": " + ann
        default = defaults[idx]
        if default is not None:
            rendered = render(default)
            if rendered:
                text += "=" + rendered
        parts.append(text)
        if posonly and idx == len(posonly) - 1:
            parts.append("/")

    if args.vararg is not None:
        text = "*" + args.vararg.arg
        ann = render(args.vararg.annotation)
        if ann:
            text += ": " + ann
        parts.append(text)
    elif args.kwonlyargs:
        parts.append("*")

    for idx, arg in enumerate(args.kwonlyargs):
        text = arg.arg
        ann = render(arg.annotation)
        if ann:
            text += ": " + ann
        default = args.kw_defaults[idx]
        if default is not None:
            rendered = render(default)
            if rendered:
                text += "=" + rendered
        parts.append(text)

    if args.kwarg is not None:
        text = "**" + args.kwarg.arg
        ann = render(args.kwarg.annotation)
        if ann:
            text += ": " + ann
        parts.append(text)

    return ", ".join(parts)

def function_signature(node):
    prefix = "async def " if isinstance(node, ast.AsyncFunctionDef) else "def "
    sig = prefix + node.name + "(" + render_args(node.args) + ")"
    returns = render(node.returns)
    if returns:
        sig += " -> " + returns
    return sig

def class_signature(node):
    bases = []
    for base in node.bases:
        rendered = render(base)
        if rendered:
            bases.append(rendered)
    for keyword in node.keywords:
        rendered = render(keyword.value)
        if keyword.arg and rendered:
            bases.append(keyword.arg + "=" + rendered)
    sig = "class " + node.name
    if bases:
        sig += "(" + ", ".join(bases) + ")"
    return sig

entries = []

def visit_body(body, parent=""):
    for node in body:
        if isinstance(node, ast.ClassDef):
            entries.append({
                "name": node.name,
                "kind": "type",
                "line": node.lineno,
                "parent": parent,
                "signature": class_signature(node),
                "doc": ast.get_docstring(node) or "",
            })
            visit_body(node.body, node.name)
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            entries.append({
                "name": node.name,
                "kind": "function",
                "line": node.lineno,
                "parent": parent,
                "signature": function_signature(node),
                "doc": ast.get_docstring(node) or "",
            })
            visit_body(node.body, node.name)

visit_body(tree.body)
print(json.dumps(entries))
`
