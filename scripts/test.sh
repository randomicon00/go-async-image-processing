#!/bin/bash

# Image Processing API Test Script
# Usage: ./test.sh [1|2|3|4|5]
# 1 = Single resize test
# 2 = Single thumbnail test
# 3 = Performance test (10 jobs)
# 4 = Performance test (50 jobs)
# 5 = Run all tests

BASE_URL="http://localhost:8081"
TEST_IMAGE="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

check_server() {
	if ! curl -s "${BASE_URL}/health" >/dev/null; then
		echo -e "${RED}Error: Server not running on ${BASE_URL}${NC}"
		echo "Start with: PORT=8081 go run main.go"
		exit 1
	fi
	echo -e "${GREEN}✓ Server is running${NC}"
}

submit_job() {
	local op=$1
	local w=$2
	local h=$3
	local fmt=$4

	local json=$(
		cat <<EOF
{"image_data": "${TEST_IMAGE}", "operation": "${op}", "width": ${w}, "height": ${h}, "output_format": "${fmt}"}
EOF
	)

	local resp=$(curl -s -X POST "${BASE_URL}/jobs" \
		-H "Content-Type: application/json" \
		-d "${json}")

	echo "$resp" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4
}

wait_for_job() {
	local jid=$1
	local max=30
	local i=1

	while [ $i -le $max ]; do
		local status=$(curl -s "${BASE_URL}/jobs/${jid}" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
		[ "$status" = "completed" ] && return 0
		[ "$status" = "failed" ] && return 1
		sleep 1
		((i++))
	done
	return 1
}

test_resize() {
	echo -e "\n${YELLOW}Test: Resize Job${NC}"
	local jid=$(submit_job "resize" 100 100 "png")
	[ -z "$jid" ] && {
		echo -e "${RED}✗ Failed to submit${NC}"
		return 1
	}
	echo "Job ID: $jid"

	if wait_for_job "$jid"; then
		echo -e "${GREEN}✓ Job completed${NC}"
		curl -s "${BASE_URL}/jobs/${jid}/download" -o test_resize.png
		echo "Image saved: test_resize.png"
		return 0
	else
		echo -e "${RED}✗ Job failed or timeout${NC}"
		return 1
	fi
}

test_thumbnail() {
	echo -e "\n${YELLOW}Test: Thumbnail Job${NC}"
	local jid=$(submit_job "thumbnail" 50 50 "png")
	[ -z "$jid" ] && {
		echo -e "${RED}✗ Failed to submit${NC}"
		return 1
	}
	echo "Job ID: $jid"

	if wait_for_job "$jid"; then
		echo -e "${GREEN}✓ Job completed${NC}"
		curl -s "${BASE_URL}/jobs/${jid}/download" -o test_thumbnail.png
		echo "Image saved: test_thumbnail.png"
		return 0
	else
		echo -e "${RED}✗ Job failed or timeout${NC}"
		return 1
	fi
}

test_performance() {
	local count=$1
	echo -e "\n${YELLOW}Test: Performance (${count} concurrent jobs)${NC}"

	local start=$(date +%s)
	local jids=()

	for i in $(seq 1 $count); do
		local jid=$(submit_job "resize" 200 200 "png")
		jids+=("$jid")
		[ $((i % 10)) -eq 0 ] && echo "  Submitted $i jobs..."
	done

	echo -e "${GREEN}✓ All ${count} jobs submitted${NC}"
	echo "Waiting for completion..."

	local completed=0
	local failed=0
	local attempts=0

	while [ $completed -lt $count ] && [ $attempts -lt 60 ]; do
		completed=0
		failed=0
		for jid in "${jids[@]}"; do
			local status=$(curl -s "${BASE_URL}/jobs/${jid}" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
			[ "$status" = "completed" ] && ((completed++))
			[ "$status" = "failed" ] && ((failed++))
		done
		echo "  Progress: $completed/$count completed, $failed failed"
		[ $completed -eq $count ] && break
		sleep 1
		((attempts++))
	done

	local end=$(date +%s)
	local duration=$((end - start))

	echo -e "\n${GREEN}Results:${NC}"
	echo "  Duration: ${duration}s"
	echo "  Completed: $completed/$count"
	echo "  Failed: $failed"
	[ $duration -gt 0 ] && echo "  Throughput: $(echo "scale=2; $count / $duration" | bc) jobs/sec"
}

# Main
echo "========================================"
echo "  Image Processing API Tests"
echo "========================================"

check_server

choice=${1:-}

if [ -z "$choice" ]; then
	echo ""
	echo "Usage: $0 [1-5]"
	echo "  1) Single resize test"
	echo "  2) Single thumbnail test"
	echo "  3) Performance test (10 jobs)"
	echo "  4) Performance test (50 jobs)"
	echo "  5) Run all tests"
	exit 0
fi

case $choice in
1) test_resize ;;
2) test_thumbnail ;;
3) test_performance 10 ;;
4) test_performance 50 ;;
5)
	test_resize
	test_thumbnail
	test_performance 10
	;;
*)
	echo "Invalid choice"
	exit 1
	;;
esac

echo -e "\n${GREEN}All tests completed!${NC}"
