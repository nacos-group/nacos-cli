package agentspec

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
)

const SharedNamespaceID = "__nacos_ai_hub__"

// AgentSpecHubInfo mirrors the server's nested HubResourceDiscoveryInfo.
type AgentSpecHubInfo struct {
	HubEntryID        string `json:"hubEntryId"`
	ResourceType      string `json:"resourceType"`
	ResourceName      string `json:"resourceName"`
	Version           string `json:"version"`
	SourceDisplayName string `json:"sourceDisplayName"`
	Author            string `json:"author"`
	DownloadCount     *int64 `json:"downloadCount,omitempty"`
	UpdateTime        *int64 `json:"updateTime,omitempty"`
}

func (item *AgentSpecListItem) applyHubMetadata() {
	if item.Hub == nil {
		return
	}
	item.HubEntryID = item.Hub.HubEntryID
	item.DisplayName = item.Hub.ResourceName
	item.SourceDisplayName = item.Hub.SourceDisplayName
	item.Version = item.Hub.Version
	item.Owner = item.Hub.Author
	item.DownloadCount = item.Hub.DownloadCount
	// A missing Hub timestamp must not be replaced by the source timestamp.
	item.UpdateTime = 0
	if item.Hub.UpdateTime != nil {
		item.UpdateTime = *item.Hub.UpdateTime
	}
}

// AgentSpecSourcePages keeps Namespace and Hub counts and pagination independent.
type AgentSpecSourcePages struct {
	Source    string                 `json:"source"`
	Namespace *AgentSpecListResponse `json:"namespace,omitempty"`
	Hub       *AgentSpecListResponse `json:"hub,omitempty"`
}

func (s *AgentSpecService) Client() *client.NacosClient { return s.client }

// ListAgentSpecsBySource reuses the existing management list in each selected Namespace.
func (s *AgentSpecService) ListAgentSpecsBySource(name string, pageNo, pageSize int, source string) (*AgentSpecSourcePages, error) {
	mode, err := config.NormalizeSkillSource(source)
	if err != nil {
		return nil, err
	}
	if pageNo < 1 || pageSize < 1 || pageSize > 100 || pageNo > math.MaxInt32/pageSize {
		return nil, fmt.Errorf("page must be positive, size must be 1-100, and paging must fit the server integer range")
	}
	if s.client.Namespace == SharedNamespaceID {
		return nil, fmt.Errorf("use agentspec-list --hub instead of selecting the internal Hub namespace")
	}
	result := &AgentSpecSourcePages{Source: mode}
	load := func(namespace, origin string) (*AgentSpecListResponse, error) {
		page, err := s.queryAgentSpecListPage(namespace, name, "", pageNo, pageSize)
		if err != nil {
			return nil, fmt.Errorf("list %s agent specs: %w", origin, err)
		}
		for i := range page.PageItems {
			page.PageItems[i].NamespaceId = namespace
			page.PageItems[i].DiscoverySource = origin
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

// IsHubEntryID distinguishes an explicit Hub selection from legacy batch names.
func IsHubEntryID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// FindAgentSpecDownloadCandidates inspects every exact-name page, retaining duplicate shares.
func (s *AgentSpecService) FindAgentSpecDownloadCandidates(name, source string) ([]AgentSpecListItem, error) {
	mode, err := config.NormalizeSkillSource(source)
	if err != nil {
		return nil, err
	}
	if s.client.Namespace == SharedNamespaceID {
		return nil, fmt.Errorf("select a Hub entry with agentspec-get <name> <hubEntryId>; keep the current Namespace unchanged")
	}
	var namespaces []string
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
	var candidates []AgentSpecListItem
	for _, namespace := range namespaces {
		for pageNo := 1; ; pageNo++ {
			page, err := s.queryAgentSpecListPage(namespace, name, "accurate", pageNo, 100)
			if err != nil {
				return nil, fmt.Errorf("find %s candidates: %w", name, err)
			}
			for _, item := range page.PageItems {
				displayName := item.Name
				if namespace == SharedNamespaceID {
					displayName = item.DisplayName
				}
				if displayName != name || (namespace != SharedNamespaceID && item.OnlineCnt == 0) {
					continue
				}
				item.NamespaceId, item.DiscoverySource = namespace, "NAMESPACE"
				if namespace == SharedNamespaceID {
					if !item.validHubIdentity() {
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
				return nil, fmt.Errorf("candidate list changed while paging; retry agentspec-get %s", name)
			}
		}
	}
	return candidates, nil
}

func (item AgentSpecListItem) validHubIdentity() bool {
	return IsHubEntryID(item.Name) && item.HubEntryID == item.Name && item.Version != "" &&
		item.DisplayName != "" && item.Hub != nil && item.Hub.ResourceType == "agentspec"
}

// GetAgentSpecCandidate pins Hub reads and preserves the authenticated business Namespace.
// AgentSpec retains its original manifest/resource layout rather than adopting Skill ZIPs.
func (s *AgentSpecService) GetAgentSpecCandidate(item AgentSpecListItem, output, version, label string) error {
	directoryName := item.Name
	if item.NamespaceId == SharedNamespaceID {
		if !item.validHubIdentity() {
			return fmt.Errorf("invalid Hub download identity")
		}
		if version != "" && version != item.Version {
			return fmt.Errorf("Hub entry %s only shares version %s; requested %s", item.Name, item.Version, version)
		}
		if label != "" && label != "latest" {
			return fmt.Errorf("Hub entry %s has one fixed shared version (%s); label %q is unavailable", item.Name, item.Version, label)
		}
		version, label, directoryName = item.Version, "", item.DisplayName
		if !filepath.IsLocal(directoryName) || filepath.Clean(directoryName) == "." {
			return fmt.Errorf("invalid Hub package directory %q", directoryName)
		}
	}
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	requestClient := *s.client
	requestClient.Namespace = item.NamespaceId
	return NewAgentSpecService(&requestClient).getAgentSpec(item.Name, output, version, label, directoryName)
}
