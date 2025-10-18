# Contributing to hyprorbit

Thank you for your interest in contributing to hyprorbit! This document provides guidelines and instructions for contributing.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```sh
   git clone https://github.com/xarcdotdev/hyprorbit.git
   cd hyprorbit
   ```
3. **Create a feature branch**:
   ```sh
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites
- Go 1.21 or higher
- Hyprland  (for testing)
- Make

### Building
```sh
# Build both binaries
make
```

### Testing
```sh
# Run tests
make test

# Run with race detection
go test -race ./...
```
s
### Commit Messages
- Use clear, descriptive commit messages
- Start with a verb in present tense (e.g., "Add", "Fix", "Update")
- Reference issue numbers when applicable

Example:
```
feat: add support for window tagging

Implements window tagging functionality to allow for more granular window identification and matching.

Fixes #123
```

### Commit Prefixes
- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Test additions or modifications
- `chore:` - Maintenance tasks

## Reporting Bugs

Use the [GitHub issue tracker](https://github.com/xarcdotdev/hyprorbit/issues) to report bugs.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
