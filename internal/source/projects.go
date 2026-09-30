package source

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Projects(ctx context.Context, paths []string) ([]Project, error) {
	projects := make([]Project, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p := Project{Name: filepath.Base(path), Path: path}
		canonical, err := filepath.Abs(path)
		if err == nil {
			canonical, err = filepath.EvalSymlinks(canonical)
		}
		if err != nil {
			p.Error = err.Error()
			projects = append(projects, p)
			continue
		}
		p.Path = canonical
		p.Name = filepath.Base(canonical)
		data, err := run(ctx, 10*time.Second, "git", "-C", canonical, "log", "-1", "--format=%H%x00%cI%x00%s%x00%B")
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			p.Error = err.Error()
			projects = append(projects, p)
			continue
		}
		fields := strings.SplitN(string(data), "\x00", 4)
		if len(fields) != 4 {
			p.Error = "git returned an invalid commit record"
			projects = append(projects, p)
			continue
		}
		p.Revision, p.Subject, p.Body = fields[0], fields[2], strings.TrimSpace(fields[3])
		p.Time, err = time.Parse(time.RFC3339, fields[1])
		if err != nil {
			p.Error = fmt.Sprintf("invalid commit time: %v", err)
		}
		projects = append(projects, p)
	}
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].Time.After(projects[j].Time) })
	return projects, nil
}
