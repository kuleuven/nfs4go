package vfs

import (
	"os"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

// IsAbs reports whether the path is absolute.
func IsAbs(path string) bool {
	return strings.HasPrefix(path, "/")
}

// Join joins any number of path elements into a single path,
// separating them with slashes.
func Join(paths ...string) string {
	if paths[0] == "/" {
		paths[0] = ""
	}

	for i := 1; i < len(paths); i++ {
		if IsAbs(paths[i]) {
			logrus.Errorf("Join(%q) called with invalid input", paths)
		}

		// Strip empty elements
		if paths[i] == "" {
			paths = append(paths[:i], paths[i+1:]...)
			i--
		}
	}

	return strings.Join(paths, string(Separator))
}

// Clean strips trailing separators from the path,
// except for the root directory, which will never be stripped.
// It also removes ./ and ../ elements.
func Clean(path string) string {
	if path == "" {
		return "."
	}

	// Strip trailing slashes.
	for path != "" && IsPathSeparator(path[len(path)-1]) {
		path = path[:len(path)-1]
	}

	if path == "" {
		return string(Separator)
	}

	// Split path elements.
	var (
		elements []string
		neg      int
	)

	for _, element := range strings.Split(path, string(Separator)) {
		if element == "" || element == "." {
			continue
		}

		if element != ".." {
			elements = append(elements, element)

			continue
		}

		// Remove the last element
		if len(elements) == 0 {
			neg++
		} else {
			elements = elements[:len(elements)-1]
		}
	}

	elements = append(slices.Repeat([]string{".."}, neg), elements...)

	switch {
	case neg == 0 && IsPathSeparator(path[0]):
		return string(Separator) + strings.Join(elements, string(Separator))
	case len(elements) == 0:
		return "."
	default:
		return strings.Join(elements, string(Separator))
	}
}

// Dir returns all but the last element of path, typically the path's directory.
// On the root directory, Dir returns "" and prints a warning.
func Dir(path string) string {
	i := len(path) - 1

	for i >= 0 && !IsPathSeparator(path[i]) {
		i--
	}

	if i < 0 || path == "/" {
		logrus.Errorf("Dir(%q) called with invalid input", path)

		return ""
	}

	if i == 0 {
		return string(Separator)
	}

	return path[:i]
}

const Separator = '/'

func IsPathSeparator(c uint8) bool {
	return c == Separator
}

// Base returns the last element of path.
// If the path is empty or root, Base returns ".".
func Base(path string) string {
	if path == "" || path == string(Separator) {
		return "."
	}

	// Strip trailing slashes.
	for path != "" && IsPathSeparator(path[len(path)-1]) {
		path = path[0 : len(path)-1]
	}

	// Find the last element
	i := len(path) - 1

	for i >= 0 && !os.IsPathSeparator(path[i]) {
		i--
	}

	if i >= 0 {
		path = path[i+1:]
	}
	// If empty now, it had only slashes.
	if path == "" {
		return "."
	}

	return path
}

func Split(path string) (string, string) {
	i := len(path) - 1

	for i >= 0 && !os.IsPathSeparator(path[i]) {
		i--
	}

	if i < 0 {
		return "", path
	}

	if i == 0 {
		return string(Separator), path[1:]
	}

	return path[:i], path[i+1:]
}
