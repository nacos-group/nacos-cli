package skill

import (
	"fmt"
	"math"

	"github.com/nacos-group/nacos-cli/internal/config"
)

const SharedNamespaceID = "__nacos_ai_hub__"

// SkillSourcePages retains independent counts and pages for each selected source.
type SkillSourcePages struct {
	Source    string             `json:"source"`
	Namespace *SkillListResponse `json:"namespace,omitempty"`
	Hub       *SkillListResponse `json:"hub,omitempty"`
}

// ListSkillsBySource reuses the management list API for both namespaces.
// It preserves governance metadata and never presents two pages as a global page.
func (s *SkillService) ListSkillsBySource(name string, pageNo, pageSize int, source string) (*SkillSourcePages, error) {
	mode, err := config.NormalizeSkillSource(source)
	if err != nil {
		return nil, err
	}
	if pageNo < 1 || pageSize < 1 || pageSize > 100 || pageNo > math.MaxInt32/pageSize {
		return nil, fmt.Errorf("page must be positive, size must be 1-100, and paging must fit the server integer range")
	}
	if s.client.Namespace == SharedNamespaceID {
		return nil, fmt.Errorf("use skill-list --hub instead of selecting the internal Hub namespace")
	}
	result := &SkillSourcePages{Source: mode}
	load := func(namespace, origin string) (*SkillListResponse, error) {
		page, err := s.listSkillPage(namespace, name, pageNo, pageSize)
		if err != nil {
			return nil, fmt.Errorf("list %s skills: %w", origin, err)
		}
		for index := range page.PageItems {
			page.PageItems[index].NamespaceID = namespace
			page.PageItems[index].DiscoverySource = origin
		}
		return page, nil
	}
	if mode != config.SkillSourceHub {
		namespace := s.client.Namespace
		if namespace == "" {
			namespace = "public"
		}
		result.Namespace, err = load(namespace, "NAMESPACE")
		if err != nil {
			return nil, err
		}
	}
	if mode != config.SkillSourceNamespace {
		result.Hub, err = load(SharedNamespaceID, "HUB")
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
