#!/bin/bash
# Test webhook locally
# Usage: ./scripts/test_hook.sh [event_type]

set -e

# Configuration
SECRET="${WHM_HOOK_SECRET:-test-secret}"
URL="${WHM2BUNNY_URL:-http://127.0.0.1:9090/hook}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Generate HMAC-SHA256 signature
generate_sig() {
    local payload="$1"
    echo -n "$payload" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}'
}

# Send webhook
send_webhook() {
    local event_name="$1"
    local payload="$2"

    echo -e "${YELLOW}Testing: ${event_name}${NC}"
    echo "Payload: $payload"
    echo ""

    local sig=$(generate_sig "$payload")

    response=$(curl -s -w "\n%{http_code}" -X POST "$URL" \
        -H "Content-Type: application/json" \
        -H "X-Whm2bunny-Signature: $sig" \
        -d "$payload" 2>&1)

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ] || [ "$http_code" = "202" ] || [ "$http_code" = "204" ]; then
        echo -e "${GREEN}Success! HTTP $http_code${NC}"
        echo "Response: $body"
    else
        echo -e "${RED}Failed! HTTP $http_code${NC}"
        echo "Response: $body"
        return 1
    fi
    echo ""
}

# Test cases
test_account_created() {
    local payload='{"event":"account_created","domain":"test.example.com","user":"testuser","plan":"basic"}'
    send_webhook "Account Created" "$payload"
}

test_account_deleted() {
    local payload='{"event":"account_deleted","domain":"test.example.com","user":"testuser"}'
    send_webhook "Account Deleted" "$payload"
}

test_account_modified() {
    local payload='{"event":"account_modified","domain":"test.example.com","new_domain":"new.example.com","user":"testuser"}'
    send_webhook "Account Modified" "$payload"
}

test_addon_created() {
    local payload='{"event":"addon_created","domain":"addon.example.com","user":"testuser","parent_domain":"example.com"}'
    send_webhook "Addon Created" "$payload"
}

test_addon_deleted() {
    local payload='{"event":"addon_deleted","domain":"addon.example.com","user":"testuser"}'
    send_webhook "Addon Deleted" "$payload"
}

test_subdomain_created() {
    local payload='{"event":"subdomain_created","subdomain":"blog","parent_domain":"example.com","full_domain":"blog.example.com","user":"testuser"}'
    send_webhook "Subdomain Created" "$payload"
}

test_subdomain_deleted() {
    local payload='{"event":"subdomain_deleted","subdomain":"blog","parent_domain":"example.com","full_domain":"blog.example.com","user":"testuser"}'
    send_webhook "Subdomain Deleted" "$payload"
}

test_invalid_signature() {
    local payload='{"event":"account_created","domain":"test.example.com","user":"testuser"}'
    echo -e "${YELLOW}Testing: Invalid Signature${NC}"
    echo "Expected: 401 Unauthorized"
    echo ""

    response=$(curl -s -w "\n%{http_code}" -X POST "$URL" \
        -H "Content-Type: application/json" \
        -H "X-Whm2bunny-Signature: invalid_signature" \
        -d "$payload" 2>&1)

    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "401" ]; then
        echo -e "${GREEN}Correctly rejected invalid signature! HTTP $http_code${NC}"
    else
        echo -e "${RED}Should have rejected invalid signature! Got HTTP $http_code${NC}"
    fi
    echo ""
}

test_health_endpoint() {
    echo -e "${YELLOW}Testing: Health Endpoint${NC}"
    echo ""

    response=$(curl -s -w "\n%{http_code}" -X GET "${URL%/hook}/health" 2>&1)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        echo -e "${GREEN}Health check passed! HTTP $http_code${NC}"
        echo "Response: $body"
    else
        echo -e "${RED}Health check failed! HTTP $http_code${NC}"
    fi
    echo ""
}

test_ready_endpoint() {
    echo -e "${YELLOW}Testing: Ready Endpoint${NC}"
    echo ""

    response=$(curl -s -w "\n%{http_code}" -X GET "${URL%/hook}/ready" 2>&1)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        echo -e "${GREEN}Ready check passed! HTTP $http_code${NC}"
        echo "Response: $body"
    else
        echo -e "${YELLOW}Service not ready yet (expected on first startup)${NC}"
        echo "Response: $body"
    fi
    echo ""
}

# Main script
main() {
    echo -e "${GREEN}=== whm2bunny Webhook Test Suite ===${NC}"
    echo "URL: $URL"
    echo "Secret: ${SECRET:0:4}***${SECRET: -4}"
    echo ""

    # Check server is running
    if ! curl -s -f "${URL%/hook}/ping" >/dev/null 2>&1; then
        echo -e "${RED}Error: Server is not responding at ${URL%/hook}${NC}"
        echo "Please start the server first:"
        echo "  ./whm2bunny serve"
        exit 1
    fi

    # Run tests based on argument
    case "${1:-all}" in
        account_created)
            test_account_created
            ;;
        account_deleted)
            test_account_deleted
            ;;
        account_modified)
            test_account_modified
            ;;
        addon_created)
            test_addon_created
            ;;
        addon_deleted)
            test_addon_deleted
            ;;
        subdomain_created)
            test_subdomain_created
            ;;
        subdomain_deleted)
            test_subdomain_deleted
            ;;
        health)
            test_health_endpoint
            ;;
        ready)
            test_ready_endpoint
            ;;
        invalid)
            test_invalid_signature
            ;;
        all)
            echo "Running all tests..."
            echo ""
            test_health_endpoint
            test_ready_endpoint
            test_account_created
            test_addon_created
            test_subdomain_created
            test_account_modified
            test_subdomain_deleted
            test_addon_deleted
            test_account_deleted
            test_invalid_signature
            echo -e "${GREEN}=== All tests completed ===${NC}"
            ;;
        *)
            echo "Usage: $0 [test_type]"
            echo ""
            echo "Test types:"
            echo "  all              Run all tests (default)"
            echo "  account_created  Test account creation webhook"
            echo "  account_deleted  Test account deletion webhook"
            echo "  account_modified Test account modification webhook"
            echo "  addon_created    Test addon domain creation"
            echo "  addon_deleted    Test addon domain deletion"
            echo "  subdomain_created Test subdomain creation"
            echo "  subdomain_deleted Test subdomain deletion"
            echo "  health           Test health endpoint"
            echo "  ready            Test ready endpoint"
            echo "  invalid          Test invalid signature rejection"
            exit 1
            ;;
    esac
}

main "$@"
