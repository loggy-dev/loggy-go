## loggy-go (Go Modules)

### Prerequisites

- Go 1.21+
- Git access to push tags to the repository

### Publishing Steps

#### Option 1: Using the publish script

```bash
./scripts/publish-loggy-go.sh v0.1.0
```

#### Option 2: Manual publishing

```bash
cd loggy-go

# Run tests
go test -v ./...

# Tidy dependencies
go mod tidy

# Commit any changes
cd ..
git add loggy-go/
git commit -m "loggy-go: prepare v0.1.0 release"

# Create and push tag (Go modules in subdirectories use subdirectory prefix)
git tag loggy-go/v0.1.0
git push origin loggy-go/v0.1.0
```

### Version Guidelines

- Go modules use semantic versioning with `v` prefix: `v0.1.0`, `v1.0.0`
- For v2+, the import path changes: `github.com/loggy-dev/loggy-go/v2`

### Verifying Publication

After pushing the tag, verify the module is available:
```bash
go list -m github.com/loggy-dev/loggy-go@v0.1.0
```

Or check on pkg.go.dev:
```
https://pkg.go.dev/github.com/loggy-dev/loggy-go@v0.1.0
```

---

## Release Checklist

Before publishing any SDK:

1. [ ] Update CHANGELOG/release notes
2. [ ] Run all tests locally
3. [ ] Update README if API changed
4. [ ] Commit all changes
5. [ ] Create git tag
6. [ ] Publish to registry
7. [ ] Verify installation works:
   - Go: `go get github.com/loggy-dev/loggy-go@<version>`
8. [ ] Update documentation if needed

---

## Troubleshooting

### Go: "module not found"

- Wait a few minutes for pkg.go.dev to index the new version
- Verify the tag was pushed: `git ls-remote --tags origin | grep loggy-go`
- Check the tag format is correct: `loggy-go/v0.1.0`

### Go: "invalid version"

- Ensure version starts with `v`: `v0.1.0` not `0.1.0`
- For subdirectory modules, use format: `loggy-go/v0.1.0`
