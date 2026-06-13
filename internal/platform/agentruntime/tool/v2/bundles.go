package v2

const (
	BundleDiagnoseCore   = "diagnose.core"
	BundleQueryGeneral   = "query.general"
	BundleSecurityAudit  = "security.audit"
	BundleOperationRead  = "operation.read"
	BundleOperationWrite = "operation.write"
	BundleDeployRecovery = "deploy.recovery"
)

type Bundle struct {
	Name  string
	Tools []string
}

type BundleSet struct {
	bundles map[string]Bundle
}

func NewBundleSet(bundles ...Bundle) *BundleSet {
	out := &BundleSet{bundles: make(map[string]Bundle, len(bundles))}
	for _, b := range bundles {
		out.bundles[b.Name] = b
	}
	return out
}

func (b *BundleSet) Tools(bundleNames ...string) []string {
	if b == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, name := range bundleNames {
		bundle, ok := b.bundles[name]
		if !ok {
			continue
		}
		for _, toolName := range bundle.Tools {
			if _, exists := seen[toolName]; exists {
				continue
			}
			seen[toolName] = struct{}{}
			out = append(out, toolName)
		}
	}
	return out
}
