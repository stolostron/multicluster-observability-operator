echo "E2E TESTS GO HERE!"

./tests/e2e/setup.sh $1
./tests/e2e/tests.sh

echo "<repo>/<component>:<tag> : $1"