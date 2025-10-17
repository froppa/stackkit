package configkit

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	uber "go.uber.org/config"
)

// Requirement describes a config requirement declared via ProvideFromKey[T](key).
//
// It identifies the YAML subtree key and the Go type expected to be populated
// from that subtree.
type Requirement struct {
	// Key is the YAML subtree key, e.g. "http" or "telemetry". Root is "".
	Key string
	// Type is a human-readable Go type string, e.g. "http.Config".
	Type string
	// PkgPath is the import path for the type's package.
	PkgPath string
}

// internal representation
type reqEntry struct {
	key  string
	t    reflect.Type // generic parameter T
	base reflect.Type // T with all pointer indirections removed
}

var (
	reqMu   sync.Mutex
	reqSeen = map[string]struct{}{}
	reqs    []reqEntry

	knownMu    sync.Mutex
	knownTypes = map[string]reflect.Type{}
)

func typeKey(key string, t reflect.Type) string { return key + "\x00" + t.String() }

// registerRequirementFor is called from ProvideFromKey to record the usage of
// a config subtree and type for discovery purposes.
func registerRequirementFor[T any](key string) {
	var zero T
	tt := reflect.TypeOf(zero)
	// If T is not a type (zero value has no dynamic type), fall back to
	// reflect.TypeOf((*T)(nil)).Elem().
	if tt == nil {
		tt = reflect.TypeOf((*T)(nil)).Elem()
	}
	registerRequirementType(key, tt)
}

// RegisterRequirement registers a configuration requirement for a given key and
// sample type. The sample may be a value or a typed nil pointer to the desired
// type. This is useful for programmatic activation without generics.
func RegisterRequirement(key string, sample any) {
	if sample == nil {
		return
	}
	tt := reflect.TypeOf(sample)
	// If it's a non-pointer zero value, use its type as is.
	registerRequirementType(key, tt)
}

// RegisterRequirementType registers a requirement using a reflect.Type.
func RegisterRequirementType(key string, tt reflect.Type) {
	if tt == nil {
		return
	}
	registerRequirementType(key, tt)
}

func registerRequirementType(key string, tt reflect.Type) {
	// Unwrap pointers for introspection.
	base := tt
	for base.Kind() == reflect.Ptr {
		base = base.Elem()
	}

	// Deduplicate by (key, T string form).
	k := typeKey(key, tt)
	reqMu.Lock()
	if _, ok := reqSeen[k]; !ok {
		reqSeen[k] = struct{}{}
		reqs = append(reqs, reqEntry{key: key, t: tt, base: base})
	}
	reqMu.Unlock()
}

// Requirements returns a snapshot of all discovered configuration requirements
// registered so far in this process.
func Requirements() []Requirement {
	reqMu.Lock()
	defer reqMu.Unlock()
	out := make([]Requirement, 0, len(reqs))
	for _, r := range reqs {
		typ := r.base
		// Package.Type or just Type if no package.
		tname := typ.Name()
		if pkg := typ.PkgPath(); pkg != "" {
			// Prefer short package name for readability if possible.
			// We can't reliably derive the short name without import, so show last path seg.
			parts := strings.Split(pkg, "/")
			short := parts[len(parts)-1]
			if short != "" {
				tname = short + "." + tname
			}
		}
		out = append(out, Requirement{
			Key:     r.key,
			Type:    tname,
			PkgPath: r.base.PkgPath(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Type < out[j].Type
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// FieldSpec describes a single field in a config struct for documentation purposes.
type FieldSpec struct {
	Path     string // YAML dot path relative to Requirement.Key
	Type     string // Go kind or type name
	Required bool   // true if validate tag contains "required"
}

// Spec returns a best-effort field specification for the given requirement.
// It infers YAML field names from `yaml` tags when present, falling back to
// lowercased field names. Embedded/inline fields are flattened.
func Spec(req Requirement) ([]FieldSpec, error) {
	reqMu.Lock()
	defer reqMu.Unlock()

	// Find the matching entry to get the reflect.Type
	var match *reqEntry
	for i := range reqs {
		r := &reqs[i]
		if r.base.PkgPath() == req.PkgPath {
			// Best effort: match by type name as well
			if r.base.Name() == trimPkg(req.Type) {
				match = r
				break
			}
		}
	}
	if match == nil {
		return nil, fmt.Errorf("config: requirement not found for %q %q", req.Key, req.Type)
	}

	var out []FieldSpec
	walkStruct(match.base, "", &out)
	return out, nil
}

func walkStruct(t reflect.Type, prefix string, out *[]FieldSpec) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		// Skip unexported
		if f.PkgPath != "" { // unexported
			continue
		}
		tag := f.Tag.Get("yaml")
		name, inline := parseYAMLTag(tag, f)
		valTag := f.Tag.Get("validate")
		required := hasRequired(valTag)

		// Determine field path
		var path string
		if inline {
			path = prefix
		} else {
			if prefix == "" {
				path = name
			} else {
				path = prefix + "." + name
			}
		}

		// If struct, recurse; otherwise record leaf
		ft := f.Type
		base := ft
		for base.Kind() == reflect.Ptr {
			base = base.Elem()
		}
		switch base.Kind() {
		case reflect.Struct:
			// Recurse into nested structs. If inline, prefix is unchanged.
			walkStruct(base, path, out)
		default:
			// Record leaf field
			if name == "-" {
				continue
			}
			kind := base.Kind().String()
			if base.Name() != "" {
				// Prefer concrete name if present
				kind = base.Name()
			}
			*out = append(*out, FieldSpec{Path: path, Type: kind, Required: required})
		}
	}
}

func parseYAMLTag(tag string, f reflect.StructField) (name string, inline bool) {
	// Prefer YAML tag. Fallback to JSON tag if YAML absent.
	tag = strings.TrimSpace(tag)
	if tag == "" {
		j := strings.TrimSpace(f.Tag.Get("json"))
		if j != "" {
			parts := strings.Split(j, ",")
			if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
				return parts[0], false
			}
		}
		// Default name is lowercased field name
		name = strings.ToLower(f.Name[:1]) + f.Name[1:]
		return name, false
	}
	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] != "" {
		name = parts[0]
	} else if len(parts) > 0 && parts[0] == "" {
		// anonymous field with inline tag like `yaml:",inline"`
		name = f.Name
	}
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "inline" {
			inline = true
			break
		}
	}
	return name, inline
}

func trimPkg(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func hasRequired(tag string) bool {
	if tag == "" {
		return false
	}
	for _, tok := range strings.Split(tag, ",") {
		if strings.TrimSpace(tok) == "required" {
			return true
		}
	}
	return false
}

// --- Known modules registry ---

// RegisterKnown registers a known module key and its config type, so tools can
// activate requirements without referencing the type directly.
// Typical usage from a module's init():
//
//	config.RegisterKnown("http", (*http.Config)(nil))
func RegisterKnown(key string, sample any) {
	if sample == nil {
		return
	}
	t := reflect.TypeOf(sample)
	if t == nil {
		return
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	knownMu.Lock()
	knownTypes[key] = t
	knownMu.Unlock()
}

// KnownType returns the reflect.Type for a known module key, if registered.
func KnownType(key string) (reflect.Type, bool) {
	knownMu.Lock()
	defer knownMu.Unlock()
	t, ok := knownTypes[key]
	return t, ok
}

// Known returns a snapshot of all known modules.
func Known() []Requirement {
	knownMu.Lock()
	defer knownMu.Unlock()
	out := make([]Requirement, 0, len(knownTypes))
	for k, t := range knownTypes {
		name := t.Name()
		if pkg := t.PkgPath(); pkg != "" {
			parts := strings.Split(pkg, "/")
			short := parts[len(parts)-1]
			if short != "" {
				name = short + "." + name
			}
		}
		out = append(out, Requirement{Key: k, Type: name, PkgPath: t.PkgPath()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// CheckResult represents the outcome of validating a single requirement against
// a configuration provider.
type CheckResult struct {
	Key     string
	Type    string
	OK      bool
	Err     error
	Issues  []string // formatted validator issues: yaml.path: rule
	Unknown []string // unknown keys detected in YAML subtree
}

// Check validates all discovered requirements against the provided YAML
// provider. It attempts to populate and validate each config subtree using the
// same rules as ProvideFromKey (including `validate` struct tags).
func Check(p *uber.YAML) []CheckResult {
	reqMu.Lock()
	snapshot := make([]reqEntry, len(reqs))
	copy(snapshot, reqs)
	reqMu.Unlock()

	out := make([]CheckResult, 0, len(snapshot))
	for _, r := range snapshot {
		// Build a pointer to base struct to populate into.
		v := reflect.New(r.base)
		// Populate from YAML subtree
		err := p.Get(r.key).Populate(v.Interface())
		var issues []string
		if err == nil {
			// Validate using the shared validator instance.
			if verr := validate.Struct(v.Interface()); verr != nil {
				issues = append(issues, formatValidationIssues(verr, r.base)...)
				err = verr
			}
		}
		// Unknown keys detection: compare YAML subtree to struct fields.
		var raw any
		if err := p.Get(r.key).Populate(&raw); err != nil {
			raw = nil
		}
		unknown := findUnknownKeys(raw, r.base, "")
		ok := err == nil && len(unknown) == 0
		tname := r.base.Name()
		if pkg := r.base.PkgPath(); pkg != "" {
			parts := strings.Split(pkg, "/")
			short := parts[len(parts)-1]
			if short != "" {
				tname = short + "." + tname
			}
		}
		out = append(out, CheckResult{Key: r.key, Type: tname, OK: ok, Err: err, Issues: issues, Unknown: unknown})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Type < out[j].Type
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ResetDiscoveryForTests clears the internal registry. Exported for tests; do not
// use in application code.
func ResetDiscoveryForTests() {
	reqMu.Lock()
	defer reqMu.Unlock()
	reqSeen = map[string]struct{}{}
	reqs = nil
}

// --- Validation issue formatting ---

// formatValidationIssues converts validator.ValidationErrors into YAML-like paths.
func formatValidationIssues(err error, root reflect.Type) []string {
	// Use reflection of struct namespace to yaml path mapping.
	// If it's not a validator error, return a single generic message.
	// We avoid importing validator types here to keep this helper decoupled.
	// When it's not a known type, we fallback to err.Error().
	// Best-effort only.
	// We detect common format substrings "Field validation for 'X' failed on the 'rule' tag".
	msg := err.Error()
	// Quick path: split by newline for multiple field errors.
	parts := strings.Split(msg, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Try to extract 'X' and 'rule'
		field, rule := extractFieldAndRule(p)
		if field != "" {
			yaml := yamlPathFromStructNS(field, root)
			if yaml == "" {
				yaml = field
			}
			out = append(out, fmt.Sprintf("%s: %s", yaml, rule))
		} else {
			out = append(out, p)
		}
	}
	return out
}

func extractFieldAndRule(s string) (field, rule string) {
	// Looks for patterns:
	// "Field validation for 'Nested.Value' failed on the 'min' tag"
	// This is fragile but good enough for surfacing rules.
	if i := strings.Index(s, "Field validation for '"); i >= 0 {
		rest := s[i+len("Field validation for '"):]
		if j := strings.Index(rest, "'"); j >= 0 {
			field = rest[:j]
			// Find rule
			if k := strings.Index(rest, "' tag"); k >= 0 {
				// walk back to opening quote before rule
				prev := rest[:k]
				if q := strings.LastIndex(prev, "'"); q >= 0 {
					rule = prev[q+1:]
				}
			}
		}
	}
	return
}

// yamlPathFromStructNS maps a validator StructNamespace (Go struct path) to a yaml-like path.
func yamlPathFromStructNS(ns string, root reflect.Type) string {
	// Unwrap pointer
	for root.Kind() == reflect.Ptr {
		root = root.Elem()
	}
	if root.Kind() != reflect.Struct || ns == "" {
		return ""
	}
	// ns may be like "Config.Nested.Value"; drop the root type name if present.
	segs := strings.Split(ns, ".")
	if len(segs) > 0 && segs[0] == root.Name() {
		segs = segs[1:]
	}
	path := make([]string, 0, len(segs))
	cur := root
	for _, name := range segs {
		// Find field by Go name
		f, ok := cur.FieldByName(name)
		if !ok {
			// Give up, return joined struct names
			return strings.Join(segs, ".")
		}
		// yaml name
		tag := f.Tag.Get("yaml")
		y, inline := parseYAMLTag(tag, f)
		if !inline {
			path = append(path, y)
		}
		// next
		t := f.Type
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		cur = t
		if cur.Kind() != reflect.Struct {
			break
		}
	}
	return strings.Join(path, ".")
}

// --- Unknown key detection ---

func findUnknownKeys(y interface{}, t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var mii map[interface{}]interface{}
	switch mm := y.(type) {
	case map[interface{}]interface{}:
		mii = mm
	case map[string]interface{}:
		mii = make(map[interface{}]interface{}, len(mm))
		for k, v := range mm {
			mii[k] = v
		}
	default:
		// Allow empty/non-map; nothing to check.
		return nil
	}

	// Build allowed fields map name->field type (flatten inlines by recursing when seen)
	allowed := map[string]reflect.StructField{}
	inlineFields := make([]reflect.StructField, 0)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		name, inline := parseYAMLTag(f.Tag.Get("yaml"), f)
		if name == "-" {
			continue
		}
		if inline {
			inlineFields = append(inlineFields, f)
			continue
		}
		allowed[name] = f
	}
	// Merge inline struct fields into allowed space for name existence checks.
	for _, f := range inlineFields {
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			continue
		}
		// add their names to allowed
		for i := 0; i < ft.NumField(); i++ {
			sf := ft.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			n, inl := parseYAMLTag(sf.Tag.Get("yaml"), sf)
			if n == "-" || inl {
				continue
			}
			allowed[n] = sf
		}
	}

	var unknown []string
	for k, v := range mii {
		ks := fmt.Sprint(k)
		if _, ok := allowed[ks]; !ok {
			// unknown at this level
			if prefix == "" {
				unknown = append(unknown, ks)
			} else {
				unknown = append(unknown, prefix+"."+ks)
			}
			continue
		}
		// Recurse only into struct fields; maps/slices allow arbitrary nested keys
		f := allowed[ks]
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			childPrefix := ks
			if prefix != "" {
				childPrefix = prefix + "." + ks
			}
			unknown = append(unknown, findUnknownKeys(v, ft, childPrefix)...)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// --- YAML skeleton generation ---

// Skeleton renders an example YAML snippet for the requirement key.
func Skeleton(req Requirement) (string, error) {
	specs, err := Spec(req)
	if err != nil {
		return "", err
	}
	// Build nested map structure from paths
	type node map[string]interface{}
	root := node{}
	for _, s := range specs {
		if s.Path == "" {
			continue
		}
		parts := strings.Split(s.Path, ".")
		cur := root
		for i, seg := range parts {
			if i == len(parts)-1 {
				// leaf
				cur[seg] = placeholderFor(s)
			} else {
				if _, ok := cur[seg]; !ok {
					cur[seg] = node{}
				}
				nxt, _ := cur[seg].(node)
				cur = nxt
			}
		}
	}
	// Render YAML
	var b strings.Builder
	if req.Key != "" {
		b.WriteString(req.Key)
		b.WriteString(":\n")
		renderNode(&b, root, 2)
	} else {
		renderNode(&b, root, 0)
	}
	return b.String(), nil
}

func renderNode(b *strings.Builder, n map[string]interface{}, indent int) {
	// Sorted keys for stable output
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pad := strings.Repeat(" ", indent)
	for _, k := range keys {
		v := n[k]
		switch vv := v.(type) {
		case map[string]interface{}:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(":\n")
			renderNode(b, vv, indent+2)
		default:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(": ")
			fmt.Fprint(b, vv)
			b.WriteString("\n")
		}
	}
}

func placeholderFor(s FieldSpec) string {
	ph := "<value>"
	t := strings.ToLower(s.Type)
	switch t {
	case "string":
		ph = "\"\""
	case "int", "int32", "int64", "uint", "uint32", "uint64":
		ph = "0"
	case "float32", "float64":
		ph = "0.0"
	case "bool":
		ph = "false"
	default:
		if strings.Contains(t, "duration") {
			ph = "\"1s\""
		}
	}
	if s.Required {
		return ph + "  # required"
	}
	return ph
}
