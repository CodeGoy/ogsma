#!/bin/bash
# config vars
wsEndpoint="ws"
addr="10.1.10.194"
port="8443"
users=("chad:password1234!" "stacy:password1234!" "john:password1234!" "jane:password1234!" "scott:password1234!")
serverKey="$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 64)"
echo "building with serverKey: ${serverKey}"
key="../certs/server.key"
cert="./certs/server.crt"
buildDir="build/"
mkdir -a "./${buildDir}"

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
    ./keystore_gen/keystore_gen  --password "${pass}" --new "${name}" --keystore "./${buildDir}${name}.keystore" --keyshare "./${buildDir}${name}.keyshare" || exit
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
            ./keystore_gen/keystore_gen --password "${pass}" --add "./${buildDir}${contactName}.keyshare" --keystore "./${buildDir}${name}.keystore"
        fi
    done
done

# generate client config.json
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    name="${USER[0]}"
    clients+="${name},"
    keystoreString=$(cat "./${buildDir}${name}.keystore")
    echo "generating config.json file for: ${name}"
    ./config_gen/config_gen --type client --keystore "${keystoreString}" --port "${port}" --addr "${addr}" --endpoint "${wsEndpoint}" --sk "${serverKey}" --opf "./${buildDir}${name}_config.json"
done

# generate server config file
echo "generating server config for clients: ${clients::-1}"
./config_gen/config_gen --type server --port "${port}" --endpoint "${wsEndpoint}" --cert "${cert}" --key "${key}" --opf "./${buildDir}server_config.json" --sk "${serverKey}" --ukfs "${clients::-1}"

# build server
cp "./${buildDir}server_config.json" "./server/config.json"


cd ./server || exit
go mod tidy && go build .
rm config.json
mv "./${programName}_server" "../${buildDir}"
cd ../

# build client for names
cd ./client || exit
go mod tidy
for (( i=0; i<num_users; i++ )); do
    IFS=':' read -ra USER <<< "${users[$i]}"
    name="${USER[0]}"
    echo "building executable for ${name}"
    cp "../${buildDir}${name}_config.json" ./config.json
    fyne build -o "../${buildDir}${name}_${programName}" .
    # go build -o "../${buildDir}${name}_${programName}" .
    ANDROID_NDK_HOME="$HOME/Android/android-ndk-r21e" fyne p --release --os android/arm64
    mv ./ogsma.apk "../${buildDir}${name}_${programName}.apk"
    rm config.json
done
cd ../