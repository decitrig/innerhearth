prod_id=innerhearthyoga
dev_id=innerhearth-testing
appcfg=${HOME}/tools/go_appengine/appcfg.py

appid=$dev_id
while getopts "p" opt; do
    case ${opt} in
	p)
	    appid=$prod_id
	    ;;
    esac
done

mv auth/salt.{go,old}
cp salt.go auth/salt.go
echo $appid
$appcfg update innerhearth \
    --oauth2 \
    -A $appid
mv auth/salt.{old,go}
