package profile

import (
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	profilesv1 "github.com/weaveworks/profiles/api/v1alpha1"

	"github.com/weaveworks/pctl/pkg/git"
)

//Artifact contains the name and objects belonging to a profile artifact
type Artifact struct {
	Objects      []runtime.Object
	Name         string
	RepoURL      string
	PathsToCopy  []string
	SparseFolder string
	Branch       string
}

// MakeArtifacts generates artifacts without owners for manual applying to
// a personal cluster.
func MakeArtifacts(sub profilesv1.ProfileSubscription, gitClient git.Git, rootDir, gitRepoNamespace, gitRepoName string) ([]Artifact, error) {
	path := sub.Spec.Path
	branchOrTag := sub.Spec.Tag
	if sub.Spec.Tag == "" {
		branchOrTag = sub.Spec.Branch
	}
	def, err := getProfileDefinition(sub.Spec.ProfileURL, branchOrTag, path, gitClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile definition: %w", err)
	}
	p := newProfile(def, sub, rootDir, gitRepoNamespace, gitRepoName)
	return p.makeArtifacts([]string{p.profileRepo()}, gitClient)
}

func (p *Profile) profileRepo() string {
	if p.subscription.Spec.Tag != "" {
		return p.subscription.Spec.ProfileURL + ":" + p.subscription.Spec.Tag
	}
	return p.subscription.Spec.ProfileURL + ":" + p.subscription.Spec.Branch + ":" + p.subscription.Spec.Path
}

func (p *Profile) makeArtifacts(profileRepos []string, gitClient git.Git) ([]Artifact, error) {
	var artifacts []Artifact
	profileRepoPath := p.subscription.Spec.Path

	for _, artifact := range p.definition.Spec.Artifacts {
		if err := artifact.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed for artifact %s: %w", artifact.Name, err)
		}
		if p.nestedName != "" {
			artifact.Name = filepath.Join(p.nestedName, artifact.Name)
		}
		a := Artifact{Name: artifact.Name}

		switch artifact.Kind {
		case profilesv1.ProfileKind:
			branchOrTag := artifact.Profile.Branch
			path := artifact.Profile.Path
			if artifact.Profile.Version != "" {
				branchOrTag = artifact.Profile.Version
				path = strings.Split(artifact.Profile.Version, "/")[0]
			}
			nestedProfileDef, err := getProfileDefinition(artifact.Profile.URL, branchOrTag, path, gitClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get profile definition %s on branch %s: %w", artifact.Profile.URL, branchOrTag, err)
			}
			nestedProfile := p.subscription.DeepCopyObject().(*profilesv1.ProfileSubscription)
			nestedProfile.Spec.ProfileURL = artifact.Profile.URL
			nestedProfile.Spec.Branch = artifact.Profile.Branch
			nestedProfile.Spec.Tag = artifact.Profile.Version
			nestedProfile.Spec.Path = artifact.Profile.Path
			if artifact.Profile.Version != "" {
				path := "."
				splitTag := strings.Split(artifact.Profile.Version, "/")
				if len(splitTag) > 1 {
					path = splitTag[0]
				}
				nestedProfile.Spec.Path = path
			}

			nestedSub := newProfile(nestedProfileDef, *nestedProfile, p.rootDir, p.gitRepositoryNamespace, p.gitRepositoryName)
			nestedSub.nestedName = artifact.Name
			profileRepoName := nestedSub.profileRepo()
			if containsKey(profileRepos, profileRepoName) {
				return nil, fmt.Errorf("recursive artifact detected: profile %s on branch %s contains an artifact that points recursively back at itself", artifact.Profile.URL, artifact.Profile.Branch)
			}
			profileRepos = append(profileRepos, profileRepoName)
			nestedArtifacts, err := nestedSub.makeArtifacts(profileRepos, gitClient)
			if err != nil {
				return nil, fmt.Errorf("failed to generate resources for nested profile %q: %w", artifact.Name, err)
			}
			artifacts = append(artifacts, nestedArtifacts...)
			p.nestedName = ""
		case profilesv1.HelmChartKind:
			helmRelease := p.makeHelmRelease(artifact, profileRepoPath)
			a.Objects = append(a.Objects, helmRelease)
			if artifact.Path != "" {
				if p.gitRepositoryNamespace == "" && p.gitRepositoryName == "" {
					return nil, fmt.Errorf("in case of local resources, the flux gitrepository object's details must be provided")
				}
				helmRelease.Spec.Chart.Spec.Chart = filepath.Join(p.rootDir, "artifacts", artifact.Name, artifact.Path)
				branch := p.subscription.Spec.Branch
				if p.subscription.Spec.Tag != "" {
					branch = p.subscription.Spec.Tag
				}
				a.RepoURL = p.subscription.Spec.ProfileURL
				a.SparseFolder = p.definition.Name
				a.Branch = branch
				a.PathsToCopy = append(a.PathsToCopy, artifact.Path)
			}
			if artifact.Chart != nil {
				helmRepository := p.makeHelmRepository(artifact.Chart.URL, artifact.Chart.Name)
				a.Objects = append(a.Objects, helmRepository)
			}
			artifacts = append(artifacts, a)
		case profilesv1.KustomizeKind:
			if p.gitRepositoryNamespace == "" && p.gitRepositoryName == "" {
				return nil, fmt.Errorf("in case of local resources, the flux gitrepository object's details must be provided")
			}
			path := filepath.Join(p.rootDir, "artifacts", artifact.Name, artifact.Path)
			a.Objects = append(a.Objects, p.makeKustomization(artifact, path))
			branch := p.subscription.Spec.Branch
			if p.subscription.Spec.Tag != "" {
				branch = p.subscription.Spec.Tag
			}
			a.RepoURL = p.subscription.Spec.ProfileURL
			a.SparseFolder = p.definition.Name
			a.Branch = branch
			a.PathsToCopy = append(a.PathsToCopy, artifact.Path)
			artifacts = append(artifacts, a)
		default:
			return nil, fmt.Errorf("artifact kind %q not recognized", artifact.Kind)
		}
	}
	return artifacts, nil
}

func containsKey(list []string, key string) bool {
	for _, value := range list {
		if value == key {
			return true
		}
	}
	return false
}

func (p *Profile) makeArtifactName(name string) string {
	// if this is a nested artifact, it's name contains a /
	if strings.Contains(name, "/") {
		name = filepath.Base(name)
	}
	return join(p.subscription.Name, p.definition.Name, name)
}

func join(s ...string) string {
	return strings.Join(s, "-")
}
