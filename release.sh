#!/bin/bash
# config vars
wsEndpoint="ws"
addr="10.1.10.194"
port="8443"
names=("chad" "stacy" "john" "jane" "scott")
passwords=("password1234!" "password1234!" "password1234!" "password1234!" "password1234!")
serverKey="Yd7OH1x6v53HSlantzjQFdWyx5GogR5v"
cert="./certs/selfsigned.crt"
key="./certs/selfsigned.key"

# script vars
num_names=${#names[@]}
programName="ogsma"
clients=""

# build required executables
cd ./config_gen/ || exit
go mod tidy && go build .
cd ../keystore_gen/ || exit
go mod tidy && go build .
cd ../

# Generate keystore files for names
for (( i=0; i<num_names; i++ )); do
  echo "generating keystore file for: ${names[$i]} with pass: ${passwords[$i]}"
  ./keystore_gen/keystore_gen  --password "${passwords[$i]}" --new "${names[$i]}" --keystore "${names[$i]}.keystore"
done

# Add contacts to keystore
for (( i=0; i<num_names; i++ )); do
  targetName="${names[$i]}"
  echo "adding contacts to keystore for: ${targetName}"
  for (( j=0; j<num_names; j++ )); do
    contactName="${names[$j]}"
    if [[ "$targetName" != "$contactName" ]]; then
      echo "adding: ${contactName} to keystore for: ${targetName}"
      ./keystore_gen/keystore_gen --password "${passwords[$i]}" --add "${contactName}.keyshare" --keystore "${targetName}.keystore"
    fi
  done
done

# generate client config.json
for (( i=0; i<num_names; i++ )); do
  clients+="${names[$i]},"
  targetName="${names[$i]}"
  keystoreString=$(cat "${targetName}.keystore")
  echo "generating config.json file for: ${targetName}"
  ./config_gen/config_gen --type client --keystore "${keystoreString}" --port "${port}" --addr "${addr}" --endpoint "${wsEndpoint}" --sk "${serverKey}" --opf "${targetName}_config.json"
done

# generate server config file
echo "generating server config for clients: ${clients::-1}"
./config_gen/config_gen --type server --port "${port}" --endpoint "${wsEndpoint}" --cert "${cert}" --key "${key}" --opf "server_config.json" --sk "${serverKey}" --ukfs "${clients::-1}"

# remove temp files
rm ./*.keyshare
rm ./*.keystore

# build server
cp server_config.json ./server/config.json
cd ./server || exit
go mod tidy && go build .
rm config.json
mv "./${programName}_server" ../
cd ../

# build client for names
cd ./client || exit
go mod tidy
for (( i=0; i<num_names; i++ )); do
    targetName="${names[$i]}"
    echo "building executable for ${targetName}"
    cp "../${targetName}_config.json" ./config.json
    go build -o "../${targetName}_${programName}" .
    ANDROID_NDK_HOME="$HOME/Android/android-ndk-r21e" fyne p --release --os android/arm64
    mv ./ogsma.apk "../${targetName}_${programName}.apk"
    rm config.json
done
cd ../