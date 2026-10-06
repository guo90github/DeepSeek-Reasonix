package memory

import (
	"os"
	"time"

	"reasonix/internal/config"
)

func archiveMemoryInDir(dir, name string) (string, error) {
	path, err := archiveInDir(dir, name)
	if err != nil {
		return "", err
	}
	if path != "" || indexContainsIn(dir, name) {
		if err := flushIndexIn(dir, indexLinesExceptIn(dir, name)); err != nil {
			return "", err
		}
	}
	return path, nil
}

func archiveInDir(dir, name string) (string, error) {
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer root.Close()

	file := name + ".md"
	if _, err := root.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := root.MkdirAll(".archive", 0o755); err != nil {
		return "", err
	}
	dest, err := archivePath(root, name, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if err := renameMemoryFile(root, file, dest); err != nil {
		return "", err
	}
	out, err := safeJoin(dir, dest)
	if err != nil {
		return "", err
	}
	return out, nil
}

// archiveStemBudget leaves room for the .md extension and the numeric
// collision suffix archivePath may append.
const archiveStemBudget = 255 - len(".md") - 8

// archiveStem bounds an archived file component the way slug bounds an active
// one. The timestamp stays at the head, so archiveTimeFromName still reads the
// archive date; only the name is truncated, with a digest keeping it distinct.
func archiveStem(name string, when time.Time) string {
	stamped := when.Format("20060102-150405.000") + "-" + name
	return config.BoundFilenameComponent(stamped, archiveStemBudget)
}
