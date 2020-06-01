echo "UNIT TESTS GO HERE!"

echo "<repo>/<component>:<tag> : $1"

git config credential.helper store
git config user.name ${GITHUB_USER}
echo "https://${GITHUB_TOKEN}:x-oauth-basic@github.com" >> ~/.git-credentials
git config -l

go test ./...