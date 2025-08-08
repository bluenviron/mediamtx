#!/bin/bash

# Test script for SRT Persistence API integration
# This verifies that srtPersistOnDisconnect can be set via API calls

set -e

echo "=== MediaMTX SRT Persistence API Test ==="
echo

# Configuration
API_PORT=9997
MEDIAMTX_CONFIG="example_srt_persistence.yml"
TEST_PATH="test_srt_stream"

echo "1. Starting MediaMTX with API enabled..."
./mediamtx $MEDIAMTX_CONFIG &
MEDIAMTX_PID=$!

# Wait for server to start
sleep 3

echo "2. Testing API path creation with srtPersistOnDisconnect..."

# Test 1: Create path with srtPersistOnDisconnect enabled
echo "   Creating path with SRT persistence enabled..."
curl -X POST "http://localhost:$API_PORT/v3/config/paths/add/$TEST_PATH" \
     -H "Content-Type: application/json" \
     -d '{
       "source": "publisher",
       "srtPersistOnDisconnect": true,
       "srtPublishPassphrase": "test123456",
       "record": false
     }' || echo "Failed to create path"

echo

# Test 2: Verify the path was created with correct settings
echo "   Verifying path configuration..."
RESPONSE=$(curl -s "http://localhost:$API_PORT/v3/config/paths/get/$TEST_PATH")
echo "   API Response: $RESPONSE"

if echo "$RESPONSE" | grep -q '"srtPersistOnDisconnect":true'; then
    echo "   ✅ SUCCESS: srtPersistOnDisconnect is set to true"
else
    echo "   ❌ ERROR: srtPersistOnDisconnect not found or incorrect"
fi

if echo "$RESPONSE" | grep -q '"srtPublishPassphrase":"test123456"'; then
    echo "   ✅ SUCCESS: srtPublishPassphrase is set correctly"
else
    echo "   ❌ ERROR: srtPublishPassphrase not found or incorrect"
fi

echo

# Test 3: Update path via PATCH
echo "   Testing PATCH to disable SRT persistence..."
curl -X PATCH "http://localhost:$API_PORT/v3/config/paths/patch/$TEST_PATH" \
     -H "Content-Type: application/json" \
     -d '{
       "srtPersistOnDisconnect": false
     }' || echo "Failed to patch path"

echo

# Test 4: Verify the update worked
echo "   Verifying path update..."
RESPONSE=$(curl -s "http://localhost:$API_PORT/v3/config/paths/get/$TEST_PATH")
echo "   Updated API Response: $RESPONSE"

if echo "$RESPONSE" | grep -q '"srtPersistOnDisconnect":false'; then
    echo "   ✅ SUCCESS: srtPersistOnDisconnect updated to false"
else
    echo "   ❌ ERROR: srtPersistOnDisconnect not updated correctly"
fi

echo

# Test 5: List all paths to see our test path
echo "   Listing all paths..."
curl -s "http://localhost:$API_PORT/v3/config/paths/list" | python3 -m json.tool || echo "Failed to list paths"

echo

# Cleanup
echo "3. Cleaning up..."
curl -X DELETE "http://localhost:$API_PORT/v3/config/paths/delete/$TEST_PATH" || echo "Failed to delete path"

echo "4. Stopping MediaMTX..."
kill $MEDIAMTX_PID 2>/dev/null || true
wait $MEDIAMTX_PID 2>/dev/null || true

echo "API test completed."