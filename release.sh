#!/bin/bash
# config vars
wsEndpoint="ws"
addr="10.1.10.194"
port="8443"
users=("chad:password1234!" "stacy:password1234!" "john:password1234!" "jane:password1234!" "scott:password1234!")
serverKey="Yd7OH1x6v53HSlantzjQFdWyx5GogR5v"
cert="./certs/selfsigned.crt"
key="./certs/selfsigned.key"

# script vars
num_users=${#users[@]}
programName="ogsma"
clients=""

# build required executables
cd ./config_gen/ || exit
go mod tidy && go build .
cd ../keystore_gen/ || exit
go mod tidy && go build .
cd ../

# Generate keystore files for names
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    name="${USER[0]}"
    pass="${USER[1]}"
    echo "generating keystore file for: ${name} with pass: ${pass}"
    ./keystore_gen/keystore_gen  --password "${pass}" --new "${name}" --keystore "${name}.keystore"
done

# Add contacts to keystore
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    pass="${USER[1]}"
    name="${USER[0]}"
    echo "adding contacts to keystore for: ${name}"
    for (( j=0; j<num_users; j++ )); do
        IFS=':' read -ra SUBUSER <<< "${users[$j]}"
        contactName="${SUBUSER[0]}"
        if [[ "$name" != "$contactName" ]]; then
            echo "adding: ${contactName} to keystore for: ${name}"
            ./keystore_gen/keystore_gen --password "${pass}" --add "${contactName}.keyshare" --keystore "${name}.keystore"
        fi
    done
done

# generate client config.json
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    name="${USER[0]}"
    clients+="${name},"
    keystoreString=$(cat "${name}.keystore")
    echo "generating config.json file for: ${name}"
    ./config_gen/config_gen --type client --keystore "${keystoreString}" --port "${port}" --addr "${addr}" --endpoint "${wsEndpoint}" --sk "${serverKey}" --opf "${name}_config.json"
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
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    name="${USER[0]}"
    echo "building executable for ${name}"
    cp "../${name}_config.json" ./config.json
    go build -o "../${name}_${programName}" .
    ANDROID_NDK_HOME="$HOME/Android/android-ndk-r21e" fyne p --release --os android/arm64
    mv ./ogsma.apk "../${name}_${programName}.apk"
    rm config.json
done
cd ../