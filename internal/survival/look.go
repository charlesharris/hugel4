package survival

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
)

// revertsCommit is the line git writes for itself when reverting. A hand-
// written "Revert ..." subject with no such line names nothing, and is counted
// as unattributed rather than guessed at.
var revertsCommit = regexp.MustCompile(`This reverts commit ([0-9a-f]{7,40})`)

// Look gathers what became of each landing, from git and from bd.
//
// This is the half that touches the world, kept apart from Grade so that what
// counts as survival stays testable without a repository. It returns facts by
// bead, and the count of reverts it could see but could not attribute.
//
// Nothing here fails the caller. A repository that has been moved, a branch
// that no longer exists, a bd that is not installed: each costs the fact it
// would have produced and nothing else, because a grading run that refuses to
// report anything is less use than one that reports what it could see.
func Look(landings []Landing) (map[string]Fact, int) {
	facts := map[string]Fact{}
	unattributed := 0

	byRepo := map[string][]Landing{}
	for _, l := range landings {
		if l.Repo != "" {
			byRepo[l.Repo] = append(byRepo[l.Repo], l)
		}
	}

	for repo, ls := range byRepo {
		// Which commits each landing put in the tree. base..sha is the exact
		// answer where the gate recorded a base; where it did not, the landed
		// sha is all there is, and a branch reverted by one of its middle
		// commits will read as work that held.
		owner := map[string]string{}
		earliest, latest := ls[0].At, time.Time{}
		branch := ""
		for _, l := range ls {
			if l.At.Before(earliest) {
				earliest = l.At
			}
			// The branch of the most recent landing, when landings disagree:
			// that is the one still being landed on, and the one a revert of
			// any of this work would be written on now.
			if l.Into != "" && l.At.After(latest) {
				branch, latest = l.Into, l.At
			}
			if l.SHA != "" {
				owner[l.SHA] = l.Bead
			}
			if l.Base != "" && l.SHA != "" {
				out, err := git(repo, "rev-list", l.Base+".."+l.SHA)
				if err != nil {
					continue
				}
				for _, sha := range strings.Fields(out) {
					owner[sha] = l.Bead
				}
			}
		}

		for _, c := range history(repo, branch, earliest) {
			m := revertsCommit.FindAllStringSubmatch(c.body, -1)
			if m == nil {
				if !strings.HasPrefix(strings.ToLower(c.subject), "revert") {
					continue
				}
				unattributed++
				continue
			}
			hit := false
			for _, g := range m {
				bead := owner[g[1]]
				if bead == "" {
					bead = byPrefix(owner, g[1])
				}
				if bead == "" {
					continue
				}
				hit = true
				// The first revert is the one that counts. A change taken out
				// twice was already not surviving after the first.
				if _, seen := facts[bead]; seen {
					continue
				}
				facts[bead] = Fact{RevertedBy: c.sha, RevertedAt: c.at, Subject: c.subject}
			}
			if !hit {
				unattributed++
			}
		}

		// Whether the bead came back. A bead the gate closed that is open again
		// is a person saying the work was not done, which grades the same edge
		// from the other side.
		for _, l := range ls {
			f := facts[l.Bead]
			if f.Status != "" || f.RevertedBy != "" {
				continue
			}
			b, err := beads.Get(repo, l.Bead)
			if err != nil {
				continue
			}
			f.Status = b.Status
			facts[l.Bead] = f
		}
	}
	return facts, unattributed
}

// commit is as much of one as grading needs.
type commit struct {
	sha     string
	at      time.Time
	subject string
	body    string
}

// history reads a branch back to the first landing being graded, oldest first,
// so that the first revert of a change is the one recorded. Reverts are
// looked for on the branch the work landed on, because that is where a revert
// of landed work goes; an abandoned branch carrying one says nothing about
// whether the change survived in the tree.
func history(repo, branch string, since time.Time) []commit {
	args := []string{"log", "--reverse", "--pretty=format:%H%x1f%ct%x1f%s%x1f%b%x1e",
		"--since", since.Format(time.RFC3339)}
	if branch != "" {
		args = append(args, branch)
	}
	out, err := git(repo, args...)
	if err != nil && branch != "" {
		// The branch may be gone, or named differently in this checkout.
		out, err = git(repo, args[:len(args)-1]...)
	}
	if err != nil {
		return nil
	}
	var cs []commit
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) < 4 {
			continue
		}
		c := commit{sha: f[0], subject: f[2], body: f[3]}
		if secs, err := strconv.ParseInt(f[1], 10, 64); err == nil {
			c.at = time.Unix(secs, 0)
		}
		cs = append(cs, c)
	}
	return cs
}

// byPrefix resolves an abbreviated sha. git writes the full one in the line it
// generates, so this is for reverts a person wrote by hand.
func byPrefix(owner map[string]string, sha string) string {
	found := ""
	for full, bead := range owner {
		if strings.HasPrefix(full, sha) {
			if found != "" && found != bead {
				return "" // ambiguous: attributing it to either would be a guess
			}
			found = bead
		}
	}
	return found
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}
