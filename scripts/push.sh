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

echo $appid
$appcfg update innerhearth \
    --oauth2 \
    -A $appid
