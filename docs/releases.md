# Releases

Pull requests and pushes to `main` run the Go build and tests on Linux and Windows. After a successful push to `main` (including a merge), semantic-release checks the commits since the last tag. Use [Conventional Commits](https://www.conventionalcommits.org/) in commits that reach `main`: `fix:` releases a patch, `feat:` releases a minor version, and a breaking-change marker requests a major version. A docs-only or chore-only push does not create a release. For a first release, make sure the history includes a release-triggering commit such as `feat: publish go-gpui`.

The release job creates a `v*` Git tag and a GitHub release with generated notes. Go users install the tagged module; the library has no standalone binaries. The job uses GitHub's `GITHUB_TOKEN` with `contents: write`, so the repository must allow that workflow to create tags and releases. Node.js is only used by the release tooling, not by go-gpui applications.

Coordinate breaking releases with Go's [major-version import path](https://go.dev/ref/mod#major-version-suffixes) rule before merging: starting with v2, update the module and imports to include the major suffix. Otherwise Go cannot consume the new major version at the existing module path.
