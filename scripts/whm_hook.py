#!/usr/bin/env python3
"""
WHM/cPanel Hook Script for whm2bunny

This script sends webhook notifications to whm2bunny when domains are
created, modified, or deleted in WHM/cPanel.

Installation:
1. Copy this script to /usr/local/cpanel/whm2bunny/whm_hook.py
2. Make it executable: chmod +x /usr/local/cpanel/whm2bunny/whm_hook.py
3. Configure webhook URL and secret in /etc/whm2bunny/config.json
4. Register in WHM: Home >> Script Hooks >> Add Script Hook

Events supported:
- createacct (account created)
- addaddondomain (addon domain added)
- parksubdomain (subdomain created)
- killacct (account terminated)
"""

import json
import hmac
import hashlib
import time
import sys
import os
import logging
from urllib.request import Request, urlopen
from urllib.error import URLError, HTTPError


# Default configuration paths
CONFIG_PATHS = [
    "/etc/whm2bunny/config.json",
    "/usr/local/cpanel/whm2bunny/config.json",
    "/var/cpanel/whm2bunny/config.json",
]

# Default values
DEFAULT_WEBHOOK_URL = "http://localhost:9090/hook"
DEFAULT_SECRET = "change-me-in-production"
DEFAULT_TIMEOUT = 30
DEFAULT_MAX_RETRIES = 3
DEFAULT_RETRY_DELAY = 2

# Log file
LOG_FILE = "/var/log/whm2bunny/hook.log"


class Config:
    """Configuration loader for whm2bunny hook"""

    def __init__(self):
        self.webhook_url = os.environ.get("WHM2BUNNY_WEBHOOK_URL", DEFAULT_WEBHOOK_URL)
        self.secret = os.environ.get("WHM2BUNNY_SECRET", DEFAULT_SECRET)
        self.timeout = int(os.environ.get("WHM2BUNNY_TIMEOUT", str(DEFAULT_TIMEOUT)))
        self.max_retries = int(os.environ.get("WHM2BUNNY_MAX_RETRIES", str(DEFAULT_MAX_RETRIES)))
        self.retry_delay = int(os.environ.get("WHM2BUNNY_RETRY_DELAY", str(DEFAULT_RETRY_DELAY)))
        self.debug = os.environ.get("WHM2BUNNY_DEBUG", "false").lower() == "true"

        # Try to load from file
        self._load_from_file()

    def _load_from_file(self):
        """Load configuration from JSON file"""
        for path in CONFIG_PATHS:
            if os.path.exists(path):
                try:
                    with open(path, 'r') as f:
                        config = json.load(f)
                        self.webhook_url = config.get("webhook_url", self.webhook_url)
                        self.secret = config.get("secret", self.secret)
                        self.timeout = config.get("timeout", self.timeout)
                        self.max_retries = config.get("max_retries", self.max_retries)
                        self.retry_delay = config.get("retry_delay", self.retry_delay)
                        self.debug = config.get("debug", self.debug)
                        return
                except (json.JSONDecodeError, IOError) as e:
                    # Continue to next path
                    pass


class Logger:
    """Simple logger for whm2bunny hook"""

    def __init__(self, debug=False):
        self.debug = debug
        self._setup_logging()

    def _setup_logging(self):
        """Setup logging to file"""
        global LOG_FILE
        # Ensure log directory exists
        log_dir = os.path.dirname(LOG_FILE)
        if log_dir and not os.path.exists(log_dir):
            try:
                os.makedirs(log_dir, mode=0o755)
            except OSError:
                # Fallback to /tmp
                LOG_FILE = "/tmp/whm2bunny_hook.log"

        logging.basicConfig(
            filename=LOG_FILE,
            level=logging.DEBUG if self.debug else logging.INFO,
            format='%(asctime)s [%(levelname)s] %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )

    def info(self, message):
        logging.info(message)
        print(message)

    def error(self, message):
        logging.error(message)
        print(message, file=sys.stderr)

    def debug(self, message):
        logging.debug(message)
        if self.debug:
            print(f"DEBUG: {message}")


class WebhookClient:
    """HTTP client for sending webhooks to whm2bunny"""

    def __init__(self, config, logger):
        self.config = config
        self.logger = logger

    def _generate_signature(self, payload):
        """Generate HMAC-SHA256 signature for payload"""
        payload_bytes = json.dumps(payload, sort_keys=True).encode('utf-8')
        return hmac.new(
            self.config.secret.encode('utf-8'),
            payload_bytes,
            hashlib.sha256
        ).hexdigest()

    def send(self, payload):
        """Send webhook with retry logic"""
        signature = self._generate_signature(payload)

        self.logger.debug(f"Sending webhook: {json.dumps(payload)}")
        self.logger.debug(f"Signature: {signature}")

        headers = {
            'Content-Type': 'application/json',
            'X-Whm2bunny-Signature': signature,
            'User-Agent': 'whm2bunny-hook/1.0'
        }

        data = json.dumps(payload).encode('utf-8')

        for attempt in range(self.config.max_retries):
            try:
                self.logger.debug(f"Attempt {attempt + 1}/{self.config.max_retries}")

                request = Request(
                    self.config.webhook_url,
                    data=data,
                    headers=headers,
                    method='POST'
                )
                request.add_header('Content-Length', len(data))

                with urlopen(request, timeout=self.config.timeout) as response:
                    response_data = response.read().decode('utf-8')
                    self.logger.info(f"Webhook sent successfully: {response.status}")
                    self.logger.debug(f"Response: {response_data}")
                    return True

            except HTTPError as e:
                self.logger.error(f"HTTP error: {e.code} - {e.reason}")
                if e.code >= 400 and e.code < 500:
                    # Client errors are not retryable
                    return False

            except URLError as e:
                self.logger.error(f"URL error: {e.reason}")

            except Exception as e:
                self.logger.error(f"Unexpected error: {str(e)}")

            # Retry after delay
            if attempt < self.config.max_retries - 1:
                time.sleep(self.config.retry_delay)

        self.logger.error("Failed to send webhook after all retries")
        return False


def handle_createacct(config, logger, client, data):
    """Handle account creation event"""
    # Extract domain from createacct data
    domain = data.get('domain')
    user = data.get('user')

    if not domain:
        logger.error("No domain in createacct data")
        return 1

    payload = {
        "event": "account_created",
        "domain": domain,
        "user": user
    }

    logger.info(f"Account created: {domain} (user: {user})")

    if client.send(payload):
        return 0
    return 1


def handle_addaddondomain(config, logger, client, data):
    """Handle addon domain creation event"""
    domain = data.get('domain')
    user = data.get('user')

    if not domain:
        logger.error("No domain in addaddondomain data")
        return 1

    payload = {
        "event": "addon_created",
        "domain": domain,
        "user": user
    }

    logger.info(f"Addon domain added: {domain} (user: {user})")

    if client.send(payload):
        return 0
    return 1


def handle_parksubdomain(config, logger, client, data):
    """Handle subdomain creation event"""
    subdomain = data.get('subdomain')
    parentdomain = data.get('rootdomain')
    user = data.get('user')

    if not subdomain:
        logger.error("No subdomain in parksubdomain data")
        return 1

    payload = {
        "event": "subdomain_created",
        "subdomain": subdomain,
        "parent_domain": parentdomain,
        "user": user
    }

    logger.info(f"Subdomain created: {subdomain}.{parentdomain} (user: {user})")

    if client.send(payload):
        return 0
    return 1


def handle_killacct(config, logger, client, data):
    """Handle account termination event"""
    domain = data.get('domain')
    user = data.get('user')

    if not domain and not user:
        logger.error("No domain or user in killacct data")
        return 1

    # If domain not provided, try to construct from user
    if not domain and user:
        # cPanel typically uses username as domain for main accounts
        domain = f"{user}.local"  # Fallback, whm2bunny will handle

    payload = {
        "event": "account_deleted",
        "domain": domain,
        "user": user
    }

    logger.info(f"Account terminated: {domain} (user: {user})")

    if client.send(payload):
        return 0
    return 1


def main():
    """Main entry point"""
    # Load configuration
    config = Config()
    logger = Logger(debug=config.debug)
    client = WebhookClient(config, logger)

    # Get event type from command line argument
    if len(sys.argv) < 2:
        logger.error("Usage: whm_hook.py <event_type> [data_json]")
        print("Usage: whm_hook.py <event_type> [data_json]")
        print("Event types: createacct, addaddondomain, parksubdomain, killacct")
        return 1

    event_type = sys.argv[1]

    # Parse data from second argument or stdin
    if len(sys.argv) >= 3:
        try:
            data = json.loads(sys.argv[2])
        except json.JSONDecodeError:
            logger.error(f"Invalid JSON data: {sys.argv[2]}")
            return 1
    else:
        # Read from stdin
        try:
            data = json.loads(sys.stdin.read())
        except json.JSONDecodeError:
            logger.error("Invalid JSON data from stdin")
            return 1

    # Route to appropriate handler
    handlers = {
        'createacct': handle_createacct,
        'addaddondomain': handle_addaddondomain,
        'parksubdomain': handle_parksubdomain,
        'killacct': handle_killacct,
    }

    handler = handlers.get(event_type)
    if not handler:
        logger.error(f"Unknown event type: {event_type}")
        return 1

    return handler(config, logger, client, data)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\nInterrupted")
        sys.exit(130)
    except Exception as e:
        print(f"Fatal error: {str(e)}", file=sys.stderr)
        sys.exit(1)
