# Interactive Add/Remove Wizards

kportal now includes interactive wizards for adding and removing port forwards directly from the running UI!

## Quick Start

Run kportal normally:
```bash
./kportal
```

From the main view:
- Press **`n`** to add a new port forward
- Press **`d`** to delete existing port forwards

## Add Forward Wizard (`n` key)

The wizard guides you through 7 steps to add a new forward:

### Step 1: Select Context
Choose from available Kubernetes contexts in your kubeconfig.

### Step 2: Select Namespace
Pick the namespace where your resource lives.

### Step 3: Select Resource Type
Three options:
- **Pod (by name prefix)** - Forward to a specific pod by prefix matching
- **Pod (by label selector)** - Forward to pods matching labels (survives restarts)
- **Service** - Most stable, load-balanced option

### Step 4: Enter Resource
- **Pod prefix**: Type a prefix like `nginx-` to match pods
- **Label selector**: Enter labels like `app=nginx,env=prod`
- **Service**: Select from a list of services

The wizard shows real-time validation and matching resources!

### Step 5: Remote Port
Enter the port number on the remote resource. The wizard displays detected ports from running containers.

### Step 6: Local Port
Enter the local port to bind to. The wizard checks availability in real-time.

### Step 7: Confirmation
Review your configuration and optionally add an alias (friendly name). Confirm to save!

### Navigation Keys

- **`↑`/`↓`** or **`j`/`k`** - Navigate options
- **`Enter`** - Confirm and proceed to next step
- **`Esc`** - Go back one step (or cancel on first step)
- **`Ctrl+C`** - Hard cancel and return to main view
- **`Backspace`** - Delete characters in text fields

## Remove Forward Wizard (`d` key)

Multi-select interface for removing forwards:

1. **Select forwards**: Use arrow keys to navigate, `Space` to toggle selection
2. **Confirm removal**: Press `Enter` and confirm your choice

### Navigation Keys

- **`↑`/`↓`** or **`j`/`k`** - Navigate forwards
- **`Space`** - Toggle selection of current forward
- **`a`** - Select all forwards
- **`n`** - Deselect all forwards
- **`Enter`** - Proceed to confirmation
- **`Esc`** - Cancel and return to main view
- **`Ctrl+C`** - Hard cancel

## Auto Hot-Reload

When you save a forward via the wizard:
1. The wizard writes to `.kportal.yaml` atomically
2. The file watcher detects the change (~100ms)
3. The manager reloads and starts the new forward
4. The UI updates automatically

No restart needed!

## Error Handling

The wizards handle errors gracefully:

- **Cluster unreachable**: Shows error but allows manual entry
- **Port conflicts**: Displays which process is using the port
- **Invalid selectors**: Shows validation errors in real-time
- **Duplicate ports**: Prevents adding forwards with conflicting ports

## Tips

### Pod Prefix Matching
When using pod prefix, you can type just the app name:
- `nginx` matches `nginx-deployment-abc123`
- `postgres` matches `postgres-statefulset-0`

### Label Selectors
Use standard Kubernetes label syntax:
- `app=nginx` - Single label
- `app=nginx,env=prod` - Multiple labels (comma-separated)
- Real-time validation shows matching pods as you type!

### Aliases
Use aliases for cleaner UI display:
- Instead of: `production/default/pod/nginx-deployment-abc123:80→8080`
- Shows as: `my-nginx:80→8080`

### Quick Selection
In list views, you can use `j`/`k` (Vim-style) or arrow keys for navigation.

## Example Workflow

Adding a forward for a PostgreSQL database:

1. Press `n` in main view
2. Select context: `production` (arrow keys + Enter)
3. Select namespace: `default` (arrow keys + Enter)
4. Select type: `Service` (arrow keys + Enter)
5. Select service: `postgres` (arrow keys + Enter)
6. Enter remote port: `5432` (type + Enter)
7. Enter local port: `5432` (type + Enter)
8. Add alias: `prod-db` (optional, type + Enter)
9. Confirm: Select "Add to .kportal.yaml" (Enter)

Done! The forward starts automatically within seconds.

## Architecture

The wizards use:
- **Config Mutator**: Safe, atomic YAML writes (temp file + rename)
- **K8s Discovery**: Lists contexts, namespaces, pods, services
- **Modal Overlays**: Wizards appear centered over the main view
- **Async Validation**: Port checks and selector validation run in background
- **Hot-Reload Integration**: File watcher picks up changes automatically

## Troubleshooting

### Wizards not appearing?
Check that kportal can connect to your Kubernetes cluster:
```bash
kubectl cluster-info
```

### Port check showing wrong status?
The port check happens asynchronously. Wait a moment after typing for validation.

### Changes not appearing?
The file watcher triggers within 100ms. If changes aren't visible, check:
1. `.kportal.yaml` was written correctly
2. No validation errors in the file
3. kportal process is still running

---

**Navigation Summary**

Main View:
- `n` - New forward wizard
- `d` - Delete forward wizard
- `Space` - Toggle forward on/off
- `↑↓/jk` - Navigate forwards
- `q` - Quit

Wizards:
- `Enter` - Next step / Confirm
- `Esc` - Previous step / Cancel
- `Ctrl+C` - Hard cancel
- `↑↓/jk` - Navigate
- `Space` - Toggle (in delete wizard)
