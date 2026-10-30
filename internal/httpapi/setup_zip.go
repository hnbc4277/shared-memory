package httpapi

import (
	"archive/zip"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"sharedmemory/internal/agents"
)

func (r *Router) appendSetupBundle(zw *zip.Writer, agent string) {
	if zw == nil {
		return
	}
	downloadsDir := ""
	if r != nil {
		downloadsDir = strings.TrimSpace(r.DownloadsDir)
	}
	if agents.NeedsHookBinaries(agent) {
		for _, name := range []string{"smhook.exe", "shared-memory-mcp.exe"} {
			p, ok := resolveSetupBinary(downloadsDir, name)
			if !ok {
				log.Printf("setup zip missing %s for agent %s", name, agent)
				continue
			}
			if err := writeZipFileFromPath(zw, name, p); err != nil {
				log.Printf("setup zip write %s: %v", name, err)
			}
		}
	}
	if !strings.EqualFold(strings.TrimSpace(agent), "cursor") {
		p, ok := resolveSetupBinary(downloadsDir, "smctl.exe")
		if !ok {
			log.Printf("setup zip missing smctl.exe for agent %s", agent)
		} else if err := writeZipFileFromPath(zw, "smctl.exe", p); err != nil {
			log.Printf("setup zip write smctl.exe: %v", err)
		}
		// Doubao connector needs MCP binary path; include it even without smhook.
		if strings.EqualFold(strings.TrimSpace(agent), "doubao") {
			if mcp, ok := resolveSetupBinary(downloadsDir, "shared-memory-mcp.exe"); ok {
				if err := writeZipFileFromPath(zw, "shared-memory-mcp.exe", mcp); err != nil {
					log.Printf("setup zip write shared-memory-mcp.exe: %v", err)
				}
			} else {
				log.Printf("setup zip missing shared-memory-mcp.exe for doubao")
			}
		}
	}
	if dirName := agents.AdapterDirName(agent); dirName != "" {
		root := resolveAdapterRoot(dirName)
		if root == "" {
			log.Printf("setup zip missing adapters/%s for agent %s", dirName, agent)
			return
		}
		if err := appendAdapterTree(zw, dirName, root); err != nil {
			log.Printf("setup zip adapters/%s: %v", dirName, err)
		}
	}
}

func resolveSetupBinary(downloadsDir, name string) (path string, ok bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return "", false
	}
	var cands []string
	add := func(p string) {
		if strings.TrimSpace(p) != "" {
			cands = append(cands, p)
		}
	}
	downloadsDir = strings.TrimSpace(downloadsDir)
	if downloadsDir != "" {
		add(filepath.Join(downloadsDir, name))
		add(filepath.Join(filepath.Dir(downloadsDir), "bin", name))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(filepath.Join(dir, name))
		add(filepath.Join(dir, "bin", name))
		add(filepath.Join(dir, "downloads", name))
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "bin", name))
		add(filepath.Join(wd, "web", "public", "downloads", name))
	}
	for _, c := range cands {
		if fileExists(c) {
			return c, true
		}
	}
	if strings.EqualFold(name, "shared-memory-mcp.exe") {
		if p, ok := resolveSetupBinary(downloadsDir, "mcp.exe"); ok {
			return p, true
		}
	}
	return "", false
}

func resolveAdapterRoot(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var cands []string
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands,
			filepath.Join(wd, "adapters", name),
			filepath.Join(wd, "..", "adapters", name),
			filepath.Join(wd, "..", "..", "adapters", name),
			filepath.Join(wd, "web", "public", "adapters", name),
		)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(dir, "adapters", name),
			filepath.Join(dir, "..", "adapters", name),
			filepath.Join(dir, "..", "..", "adapters", name),
			filepath.Join(dir, "web", "public", "adapters", name),
		)
	}
	for _, c := range cands {
		st, err := os.Stat(c)
		if err == nil && st.IsDir() {
			return filepath.Clean(c)
		}
	}
	return ""
}

func appendAdapterTree(zw *zip.Writer, dirName, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := d.Name()
		if base == "node_modules" || base == "dist" || base == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." || d.IsDir() {
			return nil
		}
		name := path.Join("adapters", dirName, filepath.ToSlash(rel))
		return writeZipFileFromPath(zw, name, p)
	})
}

func writeZipFileFromPath(zw *zip.Writer, name, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
