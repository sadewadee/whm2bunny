# WHM Hook Script Installation Guide

This guide explains how to install and configure the WHM hook script for whm2bunny.

## Prerequisites

- WHM/cPanel server with root access
- whm2bunny daemon running and accessible from the WHM server
- Webhook URL (e.g., `http://your-server:9090/hook`)
- Webhook secret (must match `WHM_HOOK_SECRET` in whm2bunny config)

## Installation Steps

### 1. Copy Script Files

```bash
# Create directory
mkdir -p /usr/local/cpanel/whm2bunny

# Copy the hook script
cp scripts/whm_hook.py /usr/local/cpanel/whm2bunny/

# Make it executable
chmod +x /usr/local/cpanel/whm2bunny/whm_hook.py
```

### 2. Configure Hook Script

Create `/etc/whm2bunny/config.json`:

```json
{
  "webhook_url": "http://your-whm2bunny-server:9090/hook",
  "secret": "your-webhook-secret-here",
  "timeout": 30,
  "max_retries": 3,
  "retry_delay": 2,
  "debug": false
}
```

Or use environment variables:

```bash
export WHM2BUNNY_WEBHOOK_URL="http://your-whm2bunny-server:9090/hook"
export WHM2BUNNY_SECRET="your-webhook-secret-here"
```

### 3. Create Log Directory

```bash
mkdir -p /var/log/whm2bunny
chown root:root /var/log/whm2bunny
chmod 755 /var/log/whm2bunny
```

### 4. Register Hooks in WHM

#### Option A: Via WHM Web Interface

1. Login to WHM as root
2. Navigate to: **Home » Script Hooks » Add Script Hook**
3. For each event, add a hook:

**Account Creation Hook:**
- **Hook Type:** `Creating an Account`
- **Stage:** `Post`
- **Script Path:** `/usr/local/cpanel/whm2bunny/whm_hook.py createacct`
- **Evaluator:** `/usr/local/cpanel/3rdparty/bin/python3`

**Addon Domain Hook:**
- **Hook Type:** `Adding an Addon Domain`
- **Stage:** `Post`
- **Script Path:** `/usr/local/cpanel/whm2bunny/whm_hook.py addaddondomain`
- **Evaluator:** `/usr/local/cpanel/3rdparty/bin/python3`

**Subdomain Hook:**
- **Hook Type:** `Parking a Subdomain`
- **Stage:** `Post`
- **Script Path:** `/usr/local/cpanel/whm2bunny/whm_hook.py parksubdomain`
- **Evaluator:** `/usr/local/cpanel/3rdparty/bin/python3`

**Account Termination Hook:**
- **Hook Type:** `Terminating an Account`
- **Stage:** `Post`
- **Script Path:** `/usr/local/cpanel/whm2bunny/whm_hook.py killacct`
- **Evaluator:** `/usr/local/cpanel/3rdparty/bin/python3`

#### Option B: Via Command Line

```bash
# Create account hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event Create --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py createacct \
  --manual /usr/local/cpanel/3rdparty/bin/python3

# Addon domain hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event AddonDomain --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py addaddondomain \
  --manual /usr/local/cpanel/3rdparty/bin/python3

# Subdomain hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --Category Whostmgr --event Parksubdomain --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py parksubdomain \
  --manual /usr/local/cpanel/3rdparty/bin/python3

# Account termination hook
/usr/local/cpanel/bin/manage_hooks add scripthook \
  --category Whostmgr --event Killacct --stage post \
  --script /usr/local/cpanel/whm2bunny/whm_hook.py killacct \
  --manual /usr/local/cpanel/3rdparty/bin/python3
```

### 5. Test the Hook

Test the hook script manually:

```bash
# Test account creation
echo '{"domain":"test.example.com","user":"testuser"}' | \
  /usr/local/cpanel/whm2bunny/whm_hook.py createacct

# Check logs
tail -f /var/log/whm2bunny/hook.log
```

### 6. Verify Hooks are Registered

```bash
/usr/local/cpanel/bin/manage_hooks list scripthooks
```

You should see all four hooks listed.

## Troubleshooting

### Script Not Executing

1. Check file permissions: `ls -la /usr/local/cpanel/whm2bunny/whm_hook.py`
2. Check Python path: `which python3` or `ls /usr/local/cpanel/3rdparty/bin/`

### Webhook Not Sending

1. Check if whm2bunny is running: `curl http://your-server:9090/health`
2. Check firewall rules between WHM server and whm2bunny server
3. Enable debug mode in config: `debug: true`
4. Check logs: `tail -f /var/log/whm2bunny/hook.log`

### Signature Verification Failed

1. Ensure secret matches between hook config and whm2bunny `WHM_HOOK_SECRET`
2. Check for extra whitespace in secret

## Removing Hooks

To remove hooks:

```bash
/usr/local/cpanel/bin/manage_hooks delete scripthook \
  --category Whostmgr --event Create --stage post

/usr/local/cpanel/bin/manage_hooks delete scripthook \
  --category Whostmgr --event AddonDomain --stage post

/usr/local/cpanel/bin/manage_hooks delete scripthook \
  --category Whostmgr --event Parksubdomain --stage post

/usr/local/cpanel/bin/manage_hooks delete scripthook \
  --category Whostmgr --event Killacct --stage post
```

## Events Reference

| Event Type | WHM Category | Trigger |
|------------|-------------|---------|
| `account_created` | Create | New cPanel account created |
| `addon_created` | AddonDomain | Addon domain added to account |
| `subdomain_created` | Parksubdomain | Subdomain created/parked |
| `account_deleted` | Killacct | Account terminated |

## Data Passed to Webhook

### Account Created / Addon Created
```json
{
  "event": "account_created",
  "domain": "example.com",
  "user": "username"
}
```

### Subdomain Created
```json
{
  "event": "subdomain_created",
  "subdomain": "blog",
  "parent_domain": "example.com",
  "user": "username"
}
```

### Account Deleted
```json
{
  "event": "account_deleted",
  "domain": "example.com",
  "user": "username"
}
```
