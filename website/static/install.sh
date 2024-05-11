case $(uname -ms) in
'Darwin arm64')
    target=darwin
    ;;
'Linux x86_64' | *)
    target=linux
    ;;
esac

bun_uri=https://github.com/klirix/jig/releases/download/master/jig-$target

curl --fail --location --progress-bar --output jig $bun_uri
chmod +x jig

if [ ! -d $HOME/.jig ]; then
  mkfir $HOME/.jig
fi

mv jig $HOME/.jig/jig

echo "Jig installed to \$HOME/.jig/jig"
echo ""
echo "Add the following to your .bashrc or .zshrc:"
echo "export PATH=\$PATH:\$HOME/.jig"