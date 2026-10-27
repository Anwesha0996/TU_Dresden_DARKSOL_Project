# helmdd

## General Overview
This component identifies dependencies that need to be downloaded for a given chart to function. This primarily includes container images, but `helmdd` is built in a way to support implementing checks for other dependencies (e.g. checking for `git clone` and later downloading the specified repository to bundle it with the chart is already implemented).

The process of identifying dependencies is as follows:
1. Render the manifests to make sure only dependencies required in the specific chart configuration are downloaded
2. Iterate over every rendered manifest and check for dependencies
   1. Unmarshal the YAML
   2. Recursively search for the specified keys (`image` for container images, `git clone` for repositories) in the resulting data structure
   3. If a key has been found, make note of the image repository + name or the Github/-lab/... URL
3. Remove duplicates from the resources identified in 2.
4. Export a YAML file containing references to the images sorted by repository name and a list of Git URLs
   - The YAML file is exported to `.local/share/helm-dtn/helmdd.yaml`

## Adding a new resource type to check for
To add a new resource type for `helmdd` to identify, the code has to be modified in some places. In the following, it will be explained using the example of Git repositories.

### 1. Adding a data structure to hold the references
In the `extractDependenciesFromManifests` function, there is a section containing definitions of data structures to hold references to resources that need to be downloaded:
```go
imagesSet := make(map[string]map[string]struct{})
```

For the purpose of adding a Git repository detection, we add the following:
```go
imagesSet := make(map[string]map[string]struct{})
uniqueGitUrls := make(map[string]string) //<- data structure for holding Git URLs
```

### 2. Passing this data structure to the dependency extraction function
With the data structure defined, we now need to amend the function signature of the `extractDependencies` function to use this data structure:
```go
func extractDependencies(data any, images map[string]map[string]struct{})
```
therefore becomes
```go
func extractDependencies(data any, images map[string]map[string]struct{}, gitUrls map[string]string)
```

The same goes for all calls to this function, which change from
```go
extractDependencies(val, images)
```
to
```go
extractDependencies(val, images, gitUrls)
```

### 3. Implement the detection logic
In `extractDependencies`, `helmdd` recursively searches for specific keys in the rendered manifest. For example, images are always preceded by the `image:` key, as the Kubernetes the API strictly expects a single string for the image field. This makes it very easy to check for as we can traverse the rendered yaml and check every string key for whether it matches `image`. The code for this check is as follows:
```go
switch v := data.(type) {
	case map[string]any: //<- v is a map of strings
		for key, val := range v { // <- check all map entries
			if key == "image" { // <- is the entry we are looking at "image"?
				if image, ok := val.(string); ok && image != "" { //<- cast the value of the "image" key to string and make sure it isn't empty
					lastInd := strings.LastIndex(image, "/") //<- split into registry/namespace and imagename:tag
					if lastInd != -1 {  // check if there is a registry/namespace part
						// image format: someregistry/somenamespace/image:tag
                        if images[image[:lastInd]] == nil { //<- check if there is already an image from this specific registry/namespace combo
							images[image[:lastInd]] = make(map[string]struct{}) // <- first image with this combination -> create new list
						}
						images[image[:lastInd]][image[lastInd+1:]] = struct{}{} // <- add image to the list of images to be pulled from this registry/namespace
					} else {
                        // image format: image:tag
						if images[""] == nil { // <- there is no image without a registry/namespace yet
							images[""] = make(map[string]struct{}) // <- create new list
						}
						images[""][image] = struct{}{} //<- add image to list
					}
				}
			}
			// Recurse into nested map values
			extractDependencies(val, images, gitUrls)
		}
```

The Git repository detection has to be done a different way, as there is no official `git-repository` key. The `git clone` commands will therefore most likely be part of a list (e.g. as part of `args` passed to a command). The check is still part of the same type switch as the check for images:

```go
case []any: //<- v is a list
		for _, item := range v { //<- no keys, as we're not looking at a map
			if val_string, ok := item.(string); ok && strings.Contains(val_string, "git clone") { //<- item is a string and contains "git clone"
				// Match the git clone command and group it by its parts (command, git url, path to clone into)
				re := regexp.MustCompile(`(git clone\s)(https://.*.git)\s(.*)\s*`)
				match := re.FindStringSubmatch(val_string)
				// Check for the existence of a path the repo should be cloned into
				if len(match) == 4 {
					// re.FindStringSubmatch returns the leftmost match at index 0, followed by the matched groups.
					// Here: ["git clone https://abc.def/hij.git some/path", "git clone", "https://abc.def/hij.git", "some/path"]
					gitUrls[match[2]] = match[3]
				} else {
					// No path specified
					gitUrls[match[2]] = ""
				}
			}
			extractDependencies(item, images, gitUrls)
		}
```
As a further example: Let's say `helmdd` should recognize `wget` calls. In this case the extension would happen in the same type check as the Git repository detection:
```go
case []any: //<- v is a list
		for _, item := range v { //<- no keys, as we're not looking at a map
			if val_string, ok := item.(string); ok && strings.Contains(val_string, "git clone") { //<- item is a string and contains "git clone"
				// Match the git clone command and group it by its parts (command, git url, path to clone into)
				re := regexp.MustCompile(`(git clone\s)(https://.*.git)\s(.*)\s*`)
				match := re.FindStringSubmatch(val_string)
				// Check for the existence of a path the repo should be cloned into
				if len(match) == 4 {
					// re.FindStringSubmatch returns the leftmost match at index 0, followed by the matched groups.
					// Here: ["git clone https://abc.def/hij.git some/path", "git clone", "https://abc.def/hij.git", "some/path"]
					gitUrls[match[2]] = match[3]
				} else {
					// No path specified
					gitUrls[match[2]] = ""
				}
			} else if val_string, ok := item.(string); ok && strings.Contains(val_string, "wget") { //<- call should contain wget
                //do something
            }
			extractDependencies(item, images, gitUrls)
		}
```

### 4. Ensure the dependencies are exported
The `` function returns a map of dependencies, which, by default looks like this:
```go
dependencies := map[string]any{
		"images": uniqueImages
	}
```
We need to add the newly created Git dependency type to this:
```go
dependencies := map[string]any{
		"images": uniqueImages,
		"git":    uniqueGitUrls, //<- the data structure we defined in 1.
	}
```

### 5. Extend the dependencies struct
The `util` package contains a definition for a `Dependencies` struct. It looks like this:
```go
type Dependencies struct {
	Images map[string][]string `yaml:"images"`
}
```

To make sure the other components are able to correctly parse the dependency list generated by `helmdd`, we need to extend this struct by adding the `Git` dependency type:
```go
package util

type Dependencies struct {
	Images map[string][]string `yaml:"images"`
	Git    map[string]string   `yaml:"git"`
}
```
With this, `helmdd` is now able to recognize `git clone` commands and mark the repositories for later download. To actually download the dependencies, some modifications to the `helmdl` component are still necessary. However, these are outlined in its respective README file.
