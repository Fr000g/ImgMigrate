package docker

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

// Client represents a Docker client
type Client struct {
	cli *client.Client
	ctx context.Context
}

// RegistryAuth contains authentication information for a Docker registry
type RegistryAuth struct {
	Username string
	Password string
	URL      string
	Insecure bool
}

// Platform represents an image platform
type Platform struct {
	OS           string
	Architecture string
	Variant      string
}

// SaveOptions represents options for saving images
type SaveOptions struct {
	UseCompression   bool
	OutputDir        string
	OperatingSystems []string
	CreateMultiArch  bool
}

// PullOptions for docker pull
type PullOptions struct {
	Platform string
}

// PushOptions for docker push
type PushOptions struct {
	RegistryAuth string
}

// NewClient creates a new Docker client
func NewClient() (*Client, error) {
	// Check if docker CLI is available
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker command not found or not executable: %v", err)
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &Client{
		cli: cli,
		ctx: ctx,
	}, nil
}

// getAuthConfig returns a base64 encoded auth config for registry authentication
func (c *Client) getAuthConfig(auth RegistryAuth) (string, error) {
	authConfig := registry.AuthConfig{
		Username: auth.Username,
		Password: auth.Password,
	}

	if auth.URL != "" {
		authConfig.ServerAddress = auth.URL
	}

	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}

// loginRegistry logs in to a Docker registry
func (c *Client) loginRegistry(auth RegistryAuth) error {
	if auth.Username == "" || auth.Password == "" || auth.URL == "" {
		return nil // Skip login if credentials are not provided
	}

	args := []string{"login", "--username", auth.Username, "--password-stdin"}
	if auth.Insecure {
		args = append(args, "--insecure")
	}
	args = append(args, auth.URL)

	cmd := exec.Command("docker", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, auth.Password)
	}()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login to registry: %v, output: %s", err, string(output))
	}

	return nil
}

// pullImage pulls a Docker image
func (c *Client) pullImage(imageName string, platform string) error {
	fmt.Printf("Pulling image %s for platform %s...\n", imageName, platform)

	args := []string{"pull"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, imageName)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// saveImage saves a Docker image to a file with optional compression
func (c *Client) saveImage(imageName string, outputPath string, useCompression bool) error {
	fmt.Printf("Saving image %s to %s...\n", imageName, outputPath)

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	args := []string{"save", imageName}
	cmd := exec.Command("docker", args...)

	var err error
	if useCompression {
		// Use compression
		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %v", err)
		}
		defer outFile.Close()

		gzWriter := gzip.NewWriter(outFile)
		defer gzWriter.Close()

		cmd.Stdout = gzWriter
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	} else {
		// No compression
		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %v", err)
		}
		defer outFile.Close()

		cmd.Stdout = outFile
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	}

	return err
}

// tagImage tags a Docker image
func (c *Client) tagImage(sourceImage, targetImage string) error {
	fmt.Printf("Tagging %s as %s...\n", sourceImage, targetImage)
	cmd := exec.Command("docker", "tag", sourceImage, targetImage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to tag image: %v, output: %s", err, string(output))
	}
	return nil
}

// pushImage pushes a Docker image to a registry
func (c *Client) pushImage(imageName string, auth RegistryAuth) error {
	fmt.Printf("Pushing image %s...\n", imageName)

	// Login to registry first if credentials are provided
	if err := c.loginRegistry(auth); err != nil {
		return err
	}

	cmd := exec.Command("docker", "push", imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// getAvailablePlatforms uses docker CLI to get the available platforms for an image
// This is a workaround for the API limitations
func (c *Client) getAvailablePlatforms(imageName string) ([]Platform, error) {
	fmt.Printf("Getting available platforms for %s...\n", imageName)

	// Pull image manifest first to ensure we have the latest info
	inspectCmd := exec.Command("docker", "manifest", "inspect", imageName)
	output, err := inspectCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect manifest: %v, output: %s", err, string(output))
	}

	var manifestData struct {
		Manifests []struct {
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
				Variant      string `json:"variant,omitempty"`
			} `json:"platform"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal(output, &manifestData); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %v", err)
	}

	var platforms []Platform
	for _, m := range manifestData.Manifests {
		platforms = append(platforms, Platform{
			OS:           m.Platform.OS,
			Architecture: m.Platform.Architecture,
			Variant:      m.Platform.Variant,
		})
	}

	return platforms, nil
}

// filterPlatforms filters platforms by OS and architecture
func (c *Client) filterPlatforms(platforms []Platform, os []string, archs []string) []Platform {
	if len(os) == 0 && len(archs) == 0 {
		return platforms
	}

	var filtered []Platform
	for _, platform := range platforms {
		// Check OS match
		osMatch := len(os) == 0
		for _, requestedOS := range os {
			if platform.OS == requestedOS {
				osMatch = true
				break
			}
		}

		// Check architecture match
		archMatch := len(archs) == 0
		for _, requestedArch := range archs {
			// Handle variant case
			archString := platform.Architecture
			if platform.Variant != "" {
				archString = fmt.Sprintf("%s/%s", platform.Architecture, platform.Variant)
			}

			if strings.Contains(archString, requestedArch) {
				archMatch = true
				break
			}
		}

		if osMatch && archMatch {
			filtered = append(filtered, platform)
		}
	}

	return filtered
}

// PullAllArchitectures pulls all available architectures for an image
func (c *Client) PullAllArchitectures(imageName string, options SaveOptions) error {
	// Get available platforms
	platforms, err := c.getAvailablePlatforms(imageName)
	if err != nil {
		return fmt.Errorf("failed to get available platforms: %v", err)
	}

	if len(platforms) == 0 {
		return fmt.Errorf("no platform information found for image %s", imageName)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(options.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Filter platforms by OS if specified
	if len(options.OperatingSystems) > 0 {
		platforms = c.filterPlatforms(platforms, options.OperatingSystems, nil)
		fmt.Printf("Filtered to %d platforms based on specified operating systems: %v\n",
			len(platforms), options.OperatingSystems)
	}

	fmt.Printf("Found %d architectures for %s\n", len(platforms), imageName)

	var taggedImages []string

	for _, platform := range platforms {
		arch := platform.Architecture
		if platform.Variant != "" {
			arch = fmt.Sprintf("%s/%s", arch, platform.Variant)
		}

		platformStr := fmt.Sprintf("%s/%s", platform.OS, arch)
		fmt.Printf("Processing image for architecture: %s\n", platformStr)

		// Pull the image for this platform
		if err := c.pullImage(imageName, platformStr); err != nil {
			fmt.Printf("Failed to pull image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Tag the image with architecture
		baseImage := strings.Split(imageName, ":")[0]
		var tag string
		if len(strings.Split(imageName, ":")) > 1 {
			tag = strings.Split(imageName, ":")[1]
		} else {
			tag = "latest"
		}

		newTag := fmt.Sprintf("%s:%s-%s", baseImage, tag, strings.Replace(platformStr, "/", "-", -1))
		if err := c.tagImage(imageName, newTag); err != nil {
			fmt.Printf("Failed to tag image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Verify the tagged image exists locally
		verifyCmd := exec.Command("docker", "image", "inspect", newTag)
		if verifyErr := verifyCmd.Run(); verifyErr != nil {
			fmt.Printf("Warning: Tagged image %s not found locally after tagging\n", newTag)
			continue
		}

		// Add to list of tagged images for multi-arch manifest
		taggedImages = append(taggedImages, newTag)

		// Wait a moment for the tag to complete
		time.Sleep(1 * time.Second)

		// Save the image with appropriate extension
		extension := ".tar"
		if options.UseCompression {
			extension = ".tar.gz"
		}

		outputPath := filepath.Join(options.OutputDir, fmt.Sprintf("%s%s", strings.Replace(newTag, "/", "-", -1), extension))
		if err := c.saveImage(newTag, outputPath, options.UseCompression); err != nil {
			fmt.Printf("Failed to save image for architecture %s: %v\n", platformStr, err)
			continue
		}

		fmt.Printf("Successfully saved image %s to %s\n", newTag, outputPath)
	}

	// Create multi-arch manifest if requested
	if options.CreateMultiArch && len(taggedImages) > 0 {
		fmt.Printf("Create multi-arch manifest option is enabled\n")
		baseImage := strings.Split(imageName, ":")[0]
		var tag string
		if len(strings.Split(imageName, ":")) > 1 {
			tag = strings.Split(imageName, ":")[1]
		} else {
			tag = "latest"
		}

		manifestTag := fmt.Sprintf("%s:%s-allarch", baseImage, tag)
		if err := c.createManifestList(imageName, manifestTag, taggedImages); err != nil {
			fmt.Printf("Failed to create multi-arch manifest: %v\n", err)
		} else {
			fmt.Printf("Successfully created multi-arch manifest %s\n", manifestTag)

			// Save the manifest image if saving locally
			if options.UseCompression {
				extension := ".tar.gz"
				outputPath := filepath.Join(options.OutputDir, fmt.Sprintf("%s%s", strings.Replace(manifestTag, "/", "-", -1), extension))
				if err := c.saveImage(manifestTag, outputPath, true); err != nil {
					fmt.Printf("Failed to save multi-arch manifest image: %v\n", err)
				} else {
					fmt.Printf("Successfully saved multi-arch manifest image to %s\n", outputPath)
				}
			}
		}
	} else if len(taggedImages) > 0 {
		fmt.Printf("Create multi-arch manifest option is disabled, skipping manifest creation\n")
	}

	return nil
}

// PullSpecificArchitectures pulls specific architectures for an image
func (c *Client) PullSpecificArchitectures(imageName string, archs []string, options SaveOptions) error {
	// Get available platforms
	platforms, err := c.getAvailablePlatforms(imageName)
	if err != nil {
		return fmt.Errorf("failed to get available platforms: %v", err)
	}

	if len(platforms) == 0 {
		return fmt.Errorf("no platform information found for image %s", imageName)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(options.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Filter platforms by OS and architecture
	platforms = c.filterPlatforms(platforms, options.OperatingSystems, archs)

	fmt.Printf("Filtering for architectures: %v and operating systems: %v\n",
		archs, options.OperatingSystems)

	if len(platforms) == 0 {
		return fmt.Errorf("no matching platforms found for the specified OS and architectures")
	}

	fmt.Printf("Found %d matching platforms after filtering\n", len(platforms))

	var taggedImages []string

	for _, platform := range platforms {
		arch := platform.Architecture
		if platform.Variant != "" {
			arch = fmt.Sprintf("%s/%s", arch, platform.Variant)
		}

		platformStr := fmt.Sprintf("%s/%s", platform.OS, arch)
		fmt.Printf("Processing image for architecture: %s\n", platformStr)

		// Pull the image for this platform
		if err := c.pullImage(imageName, platformStr); err != nil {
			fmt.Printf("Failed to pull image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Tag the image with architecture
		baseImage := strings.Split(imageName, ":")[0]
		var tag string
		if len(strings.Split(imageName, ":")) > 1 {
			tag = strings.Split(imageName, ":")[1]
		} else {
			tag = "latest"
		}

		newTag := fmt.Sprintf("%s:%s-%s", baseImage, tag, strings.Replace(platformStr, "/", "-", -1))
		if err := c.tagImage(imageName, newTag); err != nil {
			fmt.Printf("Failed to tag image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Verify the tagged image exists locally
		verifyCmd := exec.Command("docker", "image", "inspect", newTag)
		if verifyErr := verifyCmd.Run(); verifyErr != nil {
			fmt.Printf("Warning: Tagged image %s not found locally after tagging\n", newTag)
			continue
		}

		// Add to list of tagged images for multi-arch manifest
		taggedImages = append(taggedImages, newTag)

		// Wait a moment for the tag to complete
		time.Sleep(1 * time.Second)

		// Save the image with appropriate extension
		extension := ".tar"
		if options.UseCompression {
			extension = ".tar.gz"
		}

		outputPath := filepath.Join(options.OutputDir, fmt.Sprintf("%s%s", strings.Replace(newTag, "/", "-", -1), extension))
		if err := c.saveImage(newTag, outputPath, options.UseCompression); err != nil {
			fmt.Printf("Failed to save image for architecture %s: %v\n", platformStr, err)
			continue
		}

		fmt.Printf("Successfully saved image %s to %s\n", newTag, outputPath)
	}

	// Create multi-arch manifest if requested
	if options.CreateMultiArch && len(taggedImages) > 0 {
		fmt.Printf("Create multi-arch manifest option is enabled\n")
		baseImage := strings.Split(imageName, ":")[0]
		var tag string
		if len(strings.Split(imageName, ":")) > 1 {
			tag = strings.Split(imageName, ":")[1]
		} else {
			tag = "latest"
		}

		manifestTag := fmt.Sprintf("%s:%s-allarch", baseImage, tag)
		if err := c.createManifestList(imageName, manifestTag, taggedImages); err != nil {
			fmt.Printf("Failed to create multi-arch manifest: %v\n", err)
		} else {
			fmt.Printf("Successfully created multi-arch manifest %s\n", manifestTag)
		}
	} else if len(taggedImages) > 0 {
		fmt.Printf("Create multi-arch manifest option is disabled, skipping manifest creation\n")
	}

	return nil
}

// ProcessImageTask processes a single image task which can include pulling, saving, and pushing
func (c *Client) ProcessImageTask(sourceImage string, targetImage string, archs []string, allArch bool,
	saveLocally bool, options SaveOptions, auth RegistryAuth) error {

	// Handle local save options
	var localOptions SaveOptions = options
	// Only create multi-arch manifest locally if we're not pushing to remote
	if targetImage == "" {
		// Don't create manifest when only saving locally
		localOptions.CreateMultiArch = false
	}

	// Pull and save images if requested
	if saveLocally {
		if allArch {
			if err := c.PullAllArchitectures(sourceImage, localOptions); err != nil {
				return fmt.Errorf("failed to pull and save all architectures: %v", err)
			}
		} else {
			if err := c.PullSpecificArchitectures(sourceImage, archs, localOptions); err != nil {
				return fmt.Errorf("failed to pull and save specific architectures: %v", err)
			}
		}
	}

	// Push to registry if target image is specified
	if targetImage != "" {
		// Create multi-arch manifest when pushing to remote if requested
		pushOptions := options

		if allArch {
			if err := c.PushAllArchitectures(sourceImage, targetImage, auth, pushOptions); err != nil {
				return fmt.Errorf("failed to push all architectures: %v", err)
			}
		} else {
			if err := c.PushSpecificArchitectures(sourceImage, targetImage, archs, auth, pushOptions); err != nil {
				return fmt.Errorf("failed to push specific architectures: %v", err)
			}
		}
	}

	return nil
}

// PushAllArchitectures pulls all architectures from source image and pushes them to target registry
func (c *Client) PushAllArchitectures(sourceImage, targetImage string, auth RegistryAuth, options SaveOptions) error {
	// Get available platforms
	platforms, err := c.getAvailablePlatforms(sourceImage)
	if err != nil {
		return fmt.Errorf("failed to get available platforms: %v", err)
	}

	if len(platforms) == 0 {
		return fmt.Errorf("no platform information found for image %s", sourceImage)
	}

	// Filter platforms by OS if specified
	if len(options.OperatingSystems) > 0 {
		platforms = c.filterPlatforms(platforms, options.OperatingSystems, nil)
		fmt.Printf("Filtered to %d platforms based on specified operating systems: %v\n",
			len(platforms), options.OperatingSystems)
	}

	fmt.Printf("Found %d architectures for %s\n", len(platforms), sourceImage)

	var taggedImages []string

	for _, platform := range platforms {
		arch := platform.Architecture
		if platform.Variant != "" {
			arch = fmt.Sprintf("%s/%s", arch, platform.Variant)
		}

		platformStr := fmt.Sprintf("%s/%s", platform.OS, arch)
		fmt.Printf("Processing image for architecture: %s\n", platformStr)

		// Pull the image for this platform
		if err := c.pullImage(sourceImage, platformStr); err != nil {
			fmt.Printf("Failed to pull image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Tag with target image name
		targetTag := fmt.Sprintf("%s-%s", targetImage, strings.Replace(platformStr, "/", "-", -1))
		if err := c.tagImage(sourceImage, targetTag); err != nil {
			fmt.Printf("Failed to tag image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Verify the tagged image exists locally
		verifyCmd := exec.Command("docker", "image", "inspect", targetTag)
		if verifyErr := verifyCmd.Run(); verifyErr != nil {
			fmt.Printf("Warning: Tagged image %s not found locally after tagging\n", targetTag)
			continue
		}

		// Add to list of tagged images for multi-arch manifest
		taggedImages = append(taggedImages, targetTag)

		// Wait a moment for the tag to complete
		time.Sleep(1 * time.Second)

		// Push to target registry
		if err := c.pushImage(targetTag, auth); err != nil {
			fmt.Printf("Failed to push image for architecture %s: %v\n", platformStr, err)
			continue
		}

		fmt.Printf("Successfully pushed image %s\n", targetTag)
	}

	// Create multi-arch manifest if requested
	if options.CreateMultiArch && len(taggedImages) > 0 {
		fmt.Printf("Preparing to create multi-arch manifest for remote registry with %d images\n", len(taggedImages))

		// Verify all tagged images exist locally
		var validImages []string
		for _, img := range taggedImages {
			verifyCmd := exec.Command("docker", "image", "inspect", img)
			if err := verifyCmd.Run(); err == nil {
				validImages = append(validImages, img)
			} else {
				fmt.Printf("Warning: Image %s not found locally, will be excluded from manifest\n", img)
			}
		}

		if len(validImages) == 0 {
			fmt.Printf("No valid images found for manifest creation, skipping\n")
		} else {
			fmt.Printf("Creating multi-arch manifest for remote registry push\n")
			manifestTag := fmt.Sprintf("%s-allarch", targetImage)
			if err := c.createManifestList(sourceImage, manifestTag, validImages); err != nil {
				fmt.Printf("Failed to create multi-arch manifest: %v\n", err)
			} else {
				fmt.Printf("Successfully created multi-arch manifest %s\n", manifestTag)

				// Also tag the manifest with the base targetImage
				if err := c.tagImage(manifestTag, targetImage); err != nil {
					fmt.Printf("Failed to tag manifest with base image name: %v\n", err)
				} else {
					fmt.Printf("Successfully tagged manifest as %s\n", targetImage)
					// Push the base tag
					if err := c.pushImage(targetImage, auth); err != nil {
						fmt.Printf("Failed to push base manifest tag: %v\n", err)
					} else {
						fmt.Printf("Successfully pushed multi-arch image to %s\n", targetImage)
					}
				}
			}
		}
	} else {
		fmt.Printf("Multi-arch manifest creation is disabled, skipping\n")
	}

	return nil
}

// PushSpecificArchitectures pulls specific architectures from source image and pushes them to target registry
func (c *Client) PushSpecificArchitectures(sourceImage, targetImage string, archs []string, auth RegistryAuth, options SaveOptions) error {
	// Get available platforms
	platforms, err := c.getAvailablePlatforms(sourceImage)
	if err != nil {
		return fmt.Errorf("failed to get available platforms: %v", err)
	}

	if len(platforms) == 0 {
		return fmt.Errorf("no platform information found for image %s", sourceImage)
	}

	// Filter platforms by OS and architecture
	platforms = c.filterPlatforms(platforms, options.OperatingSystems, archs)

	fmt.Printf("Filtering for architectures: %v and operating systems: %v\n",
		archs, options.OperatingSystems)

	if len(platforms) == 0 {
		return fmt.Errorf("no matching platforms found for the specified OS and architectures")
	}

	fmt.Printf("Found %d matching platforms after filtering\n", len(platforms))

	var taggedImages []string

	for _, platform := range platforms {
		arch := platform.Architecture
		if platform.Variant != "" {
			arch = fmt.Sprintf("%s/%s", arch, platform.Variant)
		}

		platformStr := fmt.Sprintf("%s/%s", platform.OS, arch)
		fmt.Printf("Processing image for architecture: %s\n", platformStr)

		// Pull the image for this platform
		if err := c.pullImage(sourceImage, platformStr); err != nil {
			fmt.Printf("Failed to pull image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Tag with target image name
		targetTag := fmt.Sprintf("%s-%s", targetImage, strings.Replace(platformStr, "/", "-", -1))
		if err := c.tagImage(sourceImage, targetTag); err != nil {
			fmt.Printf("Failed to tag image for architecture %s: %v\n", platformStr, err)
			continue
		}

		// Verify the tagged image exists locally
		verifyCmd := exec.Command("docker", "image", "inspect", targetTag)
		if verifyErr := verifyCmd.Run(); verifyErr != nil {
			fmt.Printf("Warning: Tagged image %s not found locally after tagging\n", targetTag)
			continue
		}

		// Add to list of tagged images for multi-arch manifest
		taggedImages = append(taggedImages, targetTag)

		// Wait a moment for the tag to complete
		time.Sleep(1 * time.Second)

		// Push to target registry
		if err := c.pushImage(targetTag, auth); err != nil {
			fmt.Printf("Failed to push image for architecture %s: %v\n", platformStr, err)
			continue
		}

		fmt.Printf("Successfully pushed image %s\n", targetTag)
	}

	// Create multi-arch manifest if requested
	if options.CreateMultiArch && len(taggedImages) > 0 {
		fmt.Printf("Preparing to create multi-arch manifest for remote registry with %d images\n", len(taggedImages))

		// Verify all tagged images exist locally
		var validImages []string
		for _, img := range taggedImages {
			verifyCmd := exec.Command("docker", "image", "inspect", img)
			if err := verifyCmd.Run(); err == nil {
				validImages = append(validImages, img)
			} else {
				fmt.Printf("Warning: Image %s not found locally, will be excluded from manifest\n", img)
			}
		}

		if len(validImages) == 0 {
			fmt.Printf("No valid images found for manifest creation, skipping\n")
		} else {
			fmt.Printf("Creating multi-arch manifest for remote registry push\n")
			manifestTag := fmt.Sprintf("%s-allarch", targetImage)
			if err := c.createManifestList(sourceImage, manifestTag, validImages); err != nil {
				fmt.Printf("Failed to create multi-arch manifest: %v\n", err)
			} else {
				fmt.Printf("Successfully created multi-arch manifest %s\n", manifestTag)

				// Also tag the manifest with the base targetImage
				if err := c.tagImage(manifestTag, targetImage); err != nil {
					fmt.Printf("Failed to tag manifest with base image name: %v\n", err)
				} else {
					fmt.Printf("Successfully tagged manifest as %s\n", targetImage)
					// Push the base tag
					if err := c.pushImage(targetImage, auth); err != nil {
						fmt.Printf("Failed to push base manifest tag: %v\n", err)
					} else {
						fmt.Printf("Successfully pushed multi-arch image to %s\n", targetImage)
					}
				}
			}
		}
	} else {
		fmt.Printf("Multi-arch manifest creation is disabled, skipping\n")
	}

	return nil
}

// createManifestList creates a multi-architecture manifest for the tagged images
func (c *Client) createManifestList(baseImage string, targetImage string, taggedImages []string) error {
	fmt.Printf("Creating multi-architecture manifest %s with %d images...\n", targetImage, len(taggedImages))

	// Verify tagged images exist locally and get their full IDs for manifest creation
	var localImageRefs []string
	for _, img := range taggedImages {
		inspectCmd := exec.Command("docker", "image", "inspect", "--format", "{{.Id}}", img)
		output, err := inspectCmd.Output()
		if err != nil {
			fmt.Printf("Warning: Image %s not found locally, manifest creation may fail\n", img)
			// Still add the original tag to the list, in case it does exist
			localImageRefs = append(localImageRefs, img)
		} else {
			// Found local image, use it
			imageID := strings.TrimSpace(string(output))
			fmt.Printf("Found local image %s with ID %s\n", img, imageID)
			localImageRefs = append(localImageRefs, img)
		}
	}

	if len(localImageRefs) == 0 {
		return fmt.Errorf("no local images found to create manifest")
	}

	// Remove any existing manifest with this name
	removeCmd := exec.Command("docker", "manifest", "rm", targetImage)
	// Ignore errors as the manifest might not exist yet
	removeCmd.Run()

	// Create manifest
	args := []string{"manifest", "create", targetImage}
	args = append(args, localImageRefs...)

	fmt.Printf("Creating manifest with command: docker %s\n", strings.Join(args, " "))
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create manifest: %v, output: %s", err, string(output))
	}
	fmt.Printf("Successfully created manifest list locally\n")

	// Annotate manifest entries with platform info if needed
	for _, img := range localImageRefs {
		// Extract platform info from the tag
		if !strings.Contains(img, "-linux-") {
			continue // Skip if no platform info in the tag
		}

		parts := strings.Split(img, "-linux-")
		if len(parts) < 2 {
			continue
		}

		platformParts := strings.Split(parts[1], "-")
		if len(platformParts) < 1 {
			continue
		}

		// Extract architecture and variant if present
		arch := platformParts[0]
		variant := ""
		if len(platformParts) > 1 {
			variant = platformParts[1]
		}

		// Annotate with platform info
		annotateArgs := []string{"manifest", "annotate", targetImage, img, "--os", "linux", "--arch", arch}
		if variant != "" {
			annotateArgs = append(annotateArgs, "--variant", variant)
		}

		fmt.Printf("Annotating manifest with command: docker %s\n", strings.Join(annotateArgs, " "))
		annotateCmd := exec.Command("docker", annotateArgs...)
		annoOutput, annoErr := annotateCmd.CombinedOutput()
		if annoErr != nil {
			fmt.Printf("Warning: Failed to annotate manifest for %s: %v, output: %s\n", img, annoErr, string(annoOutput))
		} else {
			fmt.Printf("Annotated manifest for %s with os=linux, arch=%s, variant=%s\n", img, arch, variant)
		}
	}

	// Push manifest to registry if target contains a registry reference
	if strings.Contains(targetImage, "/") {
		fmt.Printf("Pushing multi-arch manifest to registry: %s\n", targetImage)
		pushCmd := exec.Command("docker", "manifest", "push", "--purge", targetImage)
		pushOutput, pushErr := pushCmd.CombinedOutput()
		if pushErr != nil {
			return fmt.Errorf("failed to push manifest: %v, output: %s", pushErr, string(pushOutput))
		}
		fmt.Printf("Successfully pushed manifest to registry\n")
	} else {
		// If not pushing to registry, we keep it locally
		// We could inspect it to display information
		inspectCmd := exec.Command("docker", "manifest", "inspect", targetImage)
		inspectOutput, _ := inspectCmd.CombinedOutput()
		fmt.Printf("Manifest inspect result:\n%s\n", string(inspectOutput))
	}

	return nil
}
