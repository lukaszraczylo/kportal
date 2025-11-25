# Interactive Wizards

kportal includes wizards for adding and removing port forwards from the running UI.

## ⌨️ Quick Reference

| Key | Action |
|-----|--------|
| `a` | Add new forward |
| `d` | Delete forwards |

## ➕ Add Forward Wizard

Press `a` from the main view to start the wizard.

### Steps

1. **Context** - Select Kubernetes context
2. **Namespace** - Select namespace
3. **Resource Type** - Choose pod (prefix), pod (selector), or service
4. **Resource** - Enter prefix, selector, or select service
5. **Remote Port** - Enter port on the resource
6. **Local Port** - Enter local port (validates availability)
7. **Confirm** - Review and optionally add an alias

### Navigation

| Key | Action |
|-----|--------|
| `↑↓` / `j/k` | Navigate options |
| `Enter` | Confirm and proceed |
| `Esc` | Go back / Cancel |
| `Ctrl+C` | Cancel immediately |

## 🗑️ Delete Forward Wizard

Press `d` from the main view.

### Navigation

| Key | Action |
|-----|--------|
| `↑↓` / `j/k` | Navigate |
| `Space` | Toggle selection |
| `a` | Select all |
| `n` | Deselect all |
| `Enter` | Confirm deletion |
| `Esc` | Cancel |

## 🎯 Resource Selection

### Pod by Prefix

Enter app name prefix to match pods:
- `nginx` matches `nginx-deployment-abc123`
- `postgres` matches `postgres-statefulset-0`

### Pod by Selector

Use Kubernetes label syntax:
- `app=nginx`
- `app=nginx,env=prod`

Matching pods are shown in real-time.

### Service

Select from discovered services in the namespace.

## 🔄 Auto Hot-Reload

Changes are applied automatically:
1. Wizard writes to `.kportal.yaml` atomically
2. File watcher detects change (~100ms)
3. Manager reloads and starts forward
4. UI updates

## Error Handling

The wizards handle:
- Cluster unreachable - allows manual entry
- Port conflicts - shows which process is using the port
- Invalid selectors - real-time validation
- Duplicate ports - prevents conflicts

## 🐛 Troubleshooting

### Wizard not appearing

Verify cluster connectivity:
```bash
kubectl cluster-info
```

### Port validation delayed

Port checks run asynchronously. Wait briefly after typing.

### Changes not visible

Check:
1. `.kportal.yaml` was written correctly
2. No validation errors in file
3. kportal process is running
