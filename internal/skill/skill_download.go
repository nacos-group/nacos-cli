package skill

import (
	"fmt"
	"strconv"

	"github.com/nacos-group/nacos-cli/internal/config"
)

// IsHubEntryID distinguishes an explicit Hub selection from legacy batch names.
func IsHubEntryID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// FindSkillDownloadCandidates uses the existing list API with exact name matching.
// Every matching page is inspected so a duplicate on a later page is never hidden.
func (s *SkillService) FindSkillDownloadCandidates(name, source string) ([]SkillListItem, error) {
	mode, err := config.NormalizeSkillSource(source)
	if err != nil {
		return nil, err
	}
	if s.client.Namespace == SharedNamespaceID {
		return nil, fmt.Errorf("select a Hub entry with skill-get <skillName> <hubEntryId>; keep the current Namespace unchanged")
	}
	namespaces := []string{}
	if mode != config.SkillSourceHub {
		namespace := s.client.Namespace
		if namespace == "" {
			namespace = "public"
		}
		namespaces = append(namespaces, namespace)
	}
	if mode != config.SkillSourceNamespace {
		namespaces = append(namespaces, SharedNamespaceID)
	}
	var candidates []SkillListItem
	for _, namespace := range namespaces {
		for pageNo := 1; ; pageNo++ {
			page, err := s.querySkillListPage(namespace, name, "accurate", pageNo, 100)
			if err != nil {
				return nil, fmt.Errorf("find %s candidates: %w", name, err)
			}
			for _, item := range page.PageItems {
				displayName := item.Name
				if namespace == SharedNamespaceID {
					displayName = item.DisplayName
				}
				if displayName != name {
					continue
				}
				if namespace != SharedNamespaceID && item.OnlineCnt != nil && *item.OnlineCnt == 0 {
					continue // Draft-only local resources cannot be downloaded through Client API.
				}
				item.NamespaceID, item.DiscoverySource = namespace, "NAMESPACE"
				if namespace == SharedNamespaceID {
					if !IsHubEntryID(item.Name) || item.HubEntryID != item.Name || item.Version == "" {
						return nil, fmt.Errorf("Hub returned an invalid download identity for %s", name)
					}
					item.DiscoverySource = "HUB"
				}
				candidates = append(candidates, item)
			}
			if pageNo >= page.PagesAvailable || pageNo*100 >= page.TotalCount {
				break
			}
			if len(page.PageItems) == 0 {
				return nil, fmt.Errorf("candidate list changed while paging; retry skill-get %s", name)
			}
		}
	}
	return candidates, nil
}

// FetchSkillCandidate keeps session identity/Namespace intact and pins a Hub transfer
// to the selected share version. Revocation or replacement remains server-authoritative.
func (s *SkillService) FetchSkillCandidate(item SkillListItem, version, label string) (*SkillQueryResult, error) {
	if item.NamespaceID == SharedNamespaceID {
		if !IsHubEntryID(item.Name) || item.HubEntryID != item.Name || item.Version == "" {
			return nil, fmt.Errorf("invalid Hub download identity")
		}
		if version != "" && version != item.Version {
			return nil, fmt.Errorf("Hub entry %s only shares version %s; requested %s", item.Name, item.Version, version)
		}
		if label != "" && label != "latest" {
			return nil, fmt.Errorf("Hub entry %s has one fixed shared version (%s); label %q is unavailable", item.Name, item.Version, label)
		}
		version, label = item.Version, ""
	}
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, err
	}
	// A separate request context also signs SPAS requests for the selected Namespace.
	requestClient := *s.client
	requestClient.Namespace = item.NamespaceID
	return NewSkillService(&requestClient).FetchSkill(item.Name, version, label, "")
}
