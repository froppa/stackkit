#!/bin/bash
set -e

# Build
echo "Building struktly..."
go build -o struktly ./cmd/struktly

echo ">>> RUN 1: SUCCESS CASE"
rm -rf out_success
./struktly --spec examples/struktly/petstore.yaml --policy examples/struktly/policy.yaml --out out_success
ls -l out_success
# Check for code
if [ ! -f out_success/main.go ]; then echo "FAIL: main.go missing"; exit 1; fi
if [ ! -f out_success/audit.json ]; then echo "FAIL: audit.json missing"; exit 1; fi

echo ">>> RUN 2: FAILURE CASE (Bad Code)"
rm -rf out_fail
set +e
./struktly --spec examples/struktly/petstore_fail.yaml --policy examples/struktly/policy.yaml --out out_fail
EXIT_CODE=$?
set -e
if [ $EXIT_CODE -eq 0 ]; then echo "FAIL: Should have failed"; exit 1; fi
# Check NO code
if [ -f out_fail/main.go ]; then echo "FAIL: main.go should NOT exist"; exit 1; fi
# Check Audit exists
if [ ! -f out_fail/audit.json ]; then echo "FAIL: audit.json missing"; exit 1; fi
grep "FAIL" out_fail/audit.json > /dev/null
echo "Run 2 Verified Correctly"

echo ">>> RUN 3: AMBIGUOUS CASE"
rm -rf out_ambiguous
set +e
./struktly --spec examples/struktly/petstore_ambiguous.yaml --policy examples/struktly/policy.yaml --out out_ambiguous
EXIT_CODE=$?
set -e
if [ $EXIT_CODE -eq 0 ]; then echo "FAIL: Should have failed"; exit 1; fi
grep "NEEDS_HUMAN_REVIEW" out_ambiguous/audit.json > /dev/null
echo "Run 3 Verified Correctly"

echo "ALL DEMOS PASSED"
