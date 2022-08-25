package main

// if need be, for more advanced logging, see:
//	https://github.com/golang/glog

import (
	"bytes"
	//	"flag"
	"fmt"
	"github.com/ovh/go-ovh/ovh"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ----------------------------------------------------------------------
// types

// https://api.ovh.com/console/#/me/~GET
// TODO: incomplete
type GetMe struct {
	Email     string `json:"email"`
	Country   string `json:"country"`
	FirstName string `json:"firstname"`
}

// https://api.ovh.com/console/#/me/api/application~GET
// TODO: ALPHA API
type GetMeApiApplication []int

// https://api.ovh.com/console/#/me/api/credential~GET
// TODO: ALPHA API
type GetMeApiCredential []int

// https://api.ovh.com/console/#/me/api/application/%7BapplicationId%7D~GET
//
//	Retrieve meta-data associated to an application ID
//
// TODO: ALPHA API
type GetMeApiApplicationId struct {
	ApplicationId  int    `json:"applicationId"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	ApplicationKey string `json:"applicationKey"`
}

// https://api.ovh.com/console/#/me/api/credential/%7BcredentialId%7D~GET
// TODO: ALPHA API
type GetMeApiCredentialIdRule struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type GetMeApiCredentialId struct {
	// XXX type uncertain
	AllowedIPs    []string                   `json:"allowedIPs"`
	// NOTE: 115/168 seem to correspond to the web console
	ApplicationId int                        `json:"applicationId"`
	Creation      time.Time                  `json:"creation"`
	CredentialId  int                        `json:"credentialId"`
	Expiration    time.Time                  `json:"expiration"`
	LastUse       time.Time                  `json:"lastUse"`
	OvhSupport    bool                       `json:"ovhSupport"`
	Status        string                     `json:"status"`
	Rules         []GetMeApiCredentialIdRule `json:"rules"`
}

// https://api.ovh.com/console/#/me/api/credential/%7BcredentialId%7D~DELETE
// TODO: ALPHA API
type DeleteMeApiCredentialId struct{}

// https://api.ovh.com/console/#/vps~GET
type GetVPS []string

// https://api.ovh.com/console/#/vps/%7BserviceName%7D~GET
type GetVPSNameModel struct {
	VCore  int `json:"vcore"`
	Disk   int `json:"disk"`
	Memory int `json:"memory"`
	// TODO: incomplete
}
type GetVPSName struct {
	State string          `json:"state"`
	VCore int             `json:"vcore"`
	Model GetVPSNameModel `json:"model"`
	Name  string          `json:"name"`
	// TODO: incomplete
}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/ips~GET
type GetVPSNameIps []string

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/datacenter~GET
type GetVPSNameDatacenter struct {
	LongName string `json:"longName"`
	Name     string `json:"name"`
	Country  string `json:"country"`
}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/getConsoleUrl~POST
type PostInVPSNameGetConsole struct{}
type PostOutVPSNameGetConsole string

// https://api.ovh.com/console/#/me/api/application/%7BapplicationId%7D~DELETE
type DeleteMeApiApplicationId struct{}

// https://api.ovh.com/console/#/me/sshKey~GET
type GetMeSSHKey []string

// https://api.ovh.com/console/#/me/sshKey/%7BkeyName%7D~GET
type GetMeSSHKeyName struct {
	Key     string `json:"key"`
	KeyName string `json:"keyName"`
	Default bool   `json:"default"`
}

// https://api.ovh.com/console/#/me/sshKey/%7BkeyName%7D~DELETE
type DeleteMeSSHKeyName struct{}

// https://api.ovh.com/console/#/me/sshKey~POST
type PostInMeSSHKey struct {
	Key     string `json:"key"`
	KeyName string `json:"keyName"`
}
type PostOutMeSSHKey struct{}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/images/available~GET
type GetVPSNameImagesAvailable []string

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/images/available/%7Bid%7D~GET
type GetVPSNameImagesAvailableId struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type VPSTask struct {
	DateTime time.Time `json:"datetime"`
	Id       int       `json:"id"`
	Progress int       `json:"progress"`
	State    string    `json:"state"`
	Type     string    `json:"type"`
}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/rebuild~POST
// TODO: Beta API
type PostInVPSNameRebuild struct {
	DoNotSendPassword bool   `json:"doNotSendPassword",omitempty`
	ImageId           string `json:"imageId"`
	InstallRTM        bool   `json:"installRTM,omitempty"`
	SshKey            string `json:"sshKey,omitempty"`
}
type PostOutVPSNameRebuild VPSTask

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/tasks/%7Bid%7D~GET
type GetVPSNameTasksId VPSTask

// ----------------------------------------------------------------------
// globals/constants

// ~random; this doesn't match OpenSSH's default
// way of trying keys: rsa, dsa, ecdsa, ed25519
// TODO: make this clearer
var preferedKeys = []string{
	"id_ed25519.pub",
	"id_rsa.pub",
	"id_dsa.pub",
	"id_ecdsa.pub",
}

// default OVH SSH key name
var ovhKeyName = "ovh-do-key"

// TODO: make this configurable [-t timeout]
var poolValidatedTimeout = 2 * time.Minute
var poolRebuildTimeout = 5 * time.Minute

// Wait a little for the VPS to be up before running
// resetKnownHosts(), and more generally, to attempt
// ssh(1) connections. So far, this was enough.
// TODO: make this configurable
var waitVPSUp = 5 * time.Second

var confFn = os.Getenv("HOME") + "/.ovh.conf"

// ----------------------------------------------------------------------
// functions

func isValidated(c *ovh.Client) (bool, error) {
	var y GetMe

	if err := c.Get("/me", &y); err != nil {
		serr, ok := err.(*ovh.APIError)
		if !ok || serr.Code != http.StatusForbidden {
			return false, err
		}
		return false, nil
	}

	return true, nil
}

// Used after a Ckrequest:  the CkRequest will register the new
// customer key for use in the client; hence, all (authenticated)
// requests will now fail until the credential has been validated.
func poolForValidated(c *ovh.Client) error {
	a := time.Now().Add(poolValidatedTimeout)
	for {
		time.Sleep(5 * time.Second)
		if time.Now().After(a) {
			break
		}

		// TODO: Debug log maybe?
		//		fmt.Println("Pooling...")

		ok, err := isValidated(c)
		if ok {
			return nil
		}
		if err != nil {
			return err
		}
	}

	return fmt.Errorf("Waiting for credential validation timeout")
}

func requestNewKey(c *ovh.Client) (string, error) {
	ck := c.NewCkRequest()
	ck.AddRecursiveRules(ovh.ReadWrite, "/")
	s, err := ck.Do()
	if err != nil {
		return "", err
	}

	fmt.Printf("Consumer key:   %s\n", s.ConsumerKey)
	fmt.Printf("Validatior URL: %s\n", s.ValidationURL)
	fmt.Println("Waiting for credentials to be validated...")

	if err = poolForValidated(c); err != nil {
		return "", err
	}

	return c.ConsumerKey, nil
}

// replace the customer_key in s, where s is a loaded .ini
// file ($HOME/.ovh.conf's content)
func replaceKey(s []byte, k string) []byte {
	re := regexp.MustCompile("consumer_key=.*\n")
	return re.ReplaceAll(s, []byte("consumer_key="+k+"\n"))
}

// edit the customer_key entry of the $HOME/.ovh.conf file
func editNewKey(k string) error {
	s, err := os.ReadFile(confFn)
	if err != nil {
		return err
	}
	return os.WriteFile(confFn, replaceKey(s, k), 0644)
}

// remove all expired credentials
func flushExpiredCredentials(c *ovh.Client) error {
	var xs GetMeApiCredential
	var d GetMeApiCredentialId
	var e DeleteMeApiCredentialId

	if err := c.Get("/me/api/credential", &xs); err != nil {
		return err
	}
	for _, x := range xs {
		if err := c.Get("/me/api/credential/"+strconv.Itoa(x), &d); err != nil {
			return err
		}
		if d.Status == "expired" {
			if err := c.Delete("/me/api/credential/"+strconv.Itoa(x), &e); err != nil {
				return err
			}
		}
	}

	return nil
}

// look for non-expired credentials for the app registered
// with the given client.
//
// Sorted by creation dates
func getNonExpiredCredential(c *ovh.Client) ([]*GetMeApiCredentialId, error) {
	var xs GetMeApiCredential
	var a GetMeApiCredentialId
	var b GetMeApiApplicationId

	var ys []*GetMeApiCredentialId

	if err := c.Get("/me/api/credential", &xs); err != nil {
		return nil, err
	}
	for _, x := range xs {
		if err := c.Get("/me/api/credential/"+strconv.Itoa(x), &a); err != nil {
			return nil, err
		}
		if a.Status != "expired" {
			if err := c.Get("/me/api/application/"+strconv.Itoa(a.ApplicationId), &b); err != nil {
				serr, ok := err.(*ovh.APIError)

				// application IDs refering to web console will 404;
				// just silently ignore those
				if !ok || serr.Code != http.StatusNotFound {
					return nil, err
				}
			} else if b.ApplicationKey == c.AppKey {
				z := a
				ys = append(ys, &z)
			}
		}

	}

	sort.Slice(ys, func(i, j int) bool {
		return ys[i].Creation.Before(ys[j].Creation)
	})

	return ys, nil
}

// grab a working client, cleanup expired credentials
func getClient() (*ovh.Client, error) {
	c, err := ovh.NewDefaultClient()
	if err != nil {
		return nil, fmt.Errorf("Creating new client: %s", err)
	}
	ok, err := isValidated(c)
	if err != nil {
		return nil, fmt.Errorf("Customer key validated: %s", err)
	}
	if !ok {
		log.Println("Current customer key not validated, requesting a new one")
		k, err := requestNewKey(c)
		if err != nil {
			return nil, fmt.Errorf("Customer key request: %s", err)
		}
		if err = editNewKey(k); err != nil {
			return nil, fmt.Errorf("Editing %s: %s", confFn, err)
		}
	}

	if err = flushExpiredCredentials(c); err != nil {
		return nil, fmt.Errorf("Flushing expired credentials: %s", err)
	}

	return c, nil
}

func help(n int) {
	fmt.Println("TODO")
	os.Exit(n)
}

// XXX generic experimentation; perhaps they are better approaches
// let's see where this goes.
//
// This is a bit clumsy so far, but works.
type Item interface {
	GetMeApiApplicationId | GetVPSName | GetMeSSHKeyName | GetVPSNameImagesAvailableId
}
type ItemId interface{ string | int }

func id[T any](x T) T { return x }

func forEachItem[T Item, U ItemId](c *ovh.Client, r string,
	f func(T) (bool, error), g func(U) string) error {
	var xs []U
	var y T
	if err := c.Get(r, &xs); err != nil {
		return err
	}

	for _, x := range xs {
		if err := c.Get(r+"/"+g(x), &y); err != nil {
			return err
		}
		stop, err := f(y)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}

	return nil
}

func lsApps(c *ovh.Client) error {
	return forEachItem(c,
		"/me/api/application",
		func(y GetMeApiApplicationId) (bool, error) {
			fmt.Printf("%s %d %s %s\n", y.Name, y.ApplicationId, y.Status, y.Description)
			return false, nil
		}, strconv.Itoa)
}

// NOTE: we assume a to either be an integer (ie. an ID) or
// an app name. We could be smarter.
func rmApp(c *ovh.Client, a string) error {
	var z DeleteMeApiApplicationId
	id, err := strconv.Atoi(a)

	if err != nil {
		id = -1

		err := forEachItem(c,
			"/me/api/application",
			func(y GetMeApiApplicationId) (bool, error) {
				if y.Name == a {
					id = y.ApplicationId
					return true, nil
				}
				return false, nil
			}, strconv.Itoa)

		if err != nil {
			return err
		}
	}
	if id == -1 {
		return fmt.Errorf("No application named %s", a)
	}

	// NOTE: if id doesn't exist, this will fail
	return c.Delete("/me/api/application/"+strconv.Itoa(id), &z)
}

func lsVPS(c *ovh.Client) error {
	return forEachItem(c,
		"/vps",
		func(y GetVPSName) (bool, error) {
			var ips GetVPSNameIps
			var dc GetVPSNameDatacenter
			x := y.Name

			if err := c.Get("/vps/"+x+"/ips", &ips); err != nil {
				return true, err
			}
			if err := c.Get("/vps/"+x+"/datacenter", &dc); err != nil {
				return true, err
			}
			fmt.Printf("%s:\n", x)
			fmt.Printf("  state: %s\n", y.State)
			fmt.Printf("  loc:   %s (%s)\n", dc.LongName, dc.Country)
			fmt.Printf("  ips:\n")
			for _, ip := range ips {
				fmt.Printf("    - %s\n", ip)
			}
			fmt.Printf("  disk:  %dG\n", y.Model.Disk)
			fmt.Printf("  mem:   %dM\n", y.Model.Memory)
			return false, nil
		}, id[string])
}

func getConsole(c *ovh.Client, v string) error {
	var in PostInVPSNameGetConsole
	var out PostOutVPSNameGetConsole

	if err := c.Post("/vps/"+v+"/getConsoleUrl", &in, &out); err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func getIPs(c *ovh.Client, v string) (*GetVPSNameIps, error) {
	var ips GetVPSNameIps
	err := c.Get("/vps/"+v+"/ips", &ips)
	return &ips, err
}

func foreachIPs(c *ovh.Client, v string, f func(string) error) error {
	ips, err := getIPs(c, v)
	if err != nil {
		return err
	}
	for _, ip := range *ips {
		if err := f(ip); err != nil {
			return err
		}
	}
	return nil
}

func lsIPs(c *ovh.Client, v string) error {
	return foreachIPs(c, v, func(x string) error {
		fmt.Println(x)
		return nil
	})
}

func removeKnownHosts(ip string) error {
	cmd := exec.Command("ssh-keygen", "-R", ip)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(out)
	}
	return err
}

func addKnownHosts(ip string) error {
	var outb, errb bytes.Buffer

	cmd := exec.Command("ssh-keyscan", "-H", ip)
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	// NOTE: IPs are not always all in use: for instance
	// by default, IPv6 aren't connected, so ssh-keyscan(1)
	// will fail (Network is unreachable)
	//
	// This is a bit clumsy; we could try pinging the IP
	// beforehand (but still, sshd(8) may not run there).
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: ssh-keyscan '%s' failed:\n", ip)
		xs := strings.TrimSpace(errb.String())
		for _, x := range strings.Split(xs, "\n") {
			log.Println(x)
		}
		return nil
	}

	fn := filepath.Join(os.Getenv("HOME"), "/.ssh/known_hosts")
	f, err := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err := f.WriteString(outb.String()); err != nil {
		log.Println(err)
	}

	return nil
}

func doResetKnownHosts(ip string) error {
	err := removeKnownHosts(ip)
	if err == nil {
		err = addKnownHosts(ip)
	}
	return err
}

func resetKnownHosts(c *ovh.Client, v string) error {
	return foreachIPs(c, v, doResetKnownHosts)
}

func lsKeys(c *ovh.Client) error {
	return forEachItem(c,
		"/me/sshKey",
		func(y GetMeSSHKeyName) (bool, error) {
			fmt.Printf("%s %s\n", y.KeyName, y.Key)
			return false, nil
		}, id[string])
}

func rmKey(c *ovh.Client, n string) error {
	var x DeleteMeSSHKeyName
	return c.Delete("/me/sshKey/"+n, &x)
}

func addKey(c *ovh.Client, n, v string) error {
	x := PostInMeSSHKey{v, n}
	var y PostOutMeSSHKey
	return c.Post("/me/sshKey/", &x, &y)
}

func forEachImgs(c *ovh.Client, v string,
	f func(GetVPSNameImagesAvailableId) (bool, error)) error {
	return forEachItem(c,
		"/vps/"+v+"/images/available",
		f, id[string])
}

func lsImgs(c *ovh.Client, v string) error {
	return forEachImgs(c, v,
		func(y GetVPSNameImagesAvailableId) (bool, error) {
			fmt.Printf("%s\t%s\n", y.Name, y.Id)
			return false, nil
		})
}

func splitImgName(s string) (string, float64, string, error) {
	re := regexp.MustCompile(`^([^0-9]+) ([0-9]+(?:\.[0-9]+)?)(.*)$`)
	xs := re.FindStringSubmatch(s)
	if xs == nil || len(xs) == 0 {
		return "", -1., "", fmt.Errorf("Invalid version name: '%s'", s)
	}

	// should never fail given regexp
	v, err := strconv.ParseFloat(xs[2], 64)
	if err != nil {
		return "", -1., "", fmt.Errorf("Invalid version number: '%s' (%s)", s, xs[2])
	}

	return xs[1], v, xs[3], nil
}

// if r exactly matches an image name, return this image
// if r as a regexp match an image name, use the image with
// biggest version number
func getMatchingImg(c *ovh.Client, v string, r string) (string, string, error) {
	a := -1.
	id := ""
	name := ""
	e := ""
	err := forEachImgs(c, v,
		func(y GetVPSNameImagesAvailableId) (bool, error) {
			// exact match
			if y.Name == r {
				id = y.Id
				name = y.Name
				return true, nil
			}
			ok, err := regexp.MatchString(r, y.Name)
			if err != nil {
				return true, err
			}
			if ok {
				_, b, f, err := splitImgName(y.Name)
				if err != nil {
					return true, err
				}
				// NOTE: arbitrarily always choose
				// version who do *not* have extras.
				// e.g. "Debian 10" will be selected
				// in front of "Debian 10 - Docker"
				if (b == a && e != "" && f == "") || b > a {
					a = b
					id = y.Id
					name = y.Name
					e = f
				}
			}
			return false, nil
		})
	return id, name, err
}

// read a SSH key from $HOME/.ssh
func readSSHKey() (string, error) {
	for _, fn := range preferedKeys {
		p := filepath.Join(os.Getenv("HOME"), "/.ssh/", fn)
		s, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}

		return strings.TrimSuffix(string(s), "\n"), nil
	}
	return "", fmt.Errorf("No SSH key found!")
}

// e.g. f4b12e37-4241-4301-aadf-85ae34cdd6a9
func isImgId(s string) bool {
	h := "[0-9a-fA-F]"
	r := fmt.Sprintf("^%s{8}-%s{4}-%s{4}-%s{4}-%s{12}$", h, h, h, h, h)
	return regexp.MustCompile(r).MatchString(s)
}

func poolTask(c *ovh.Client, v string, i int) error {
	done := map[string]bool{
		"cancelled": true,
		"done":      true,
		"error":     true,
	}
	a := time.Now().Add(poolRebuildTimeout)
	for {
		time.Sleep(5 * time.Second)
		if time.Now().After(a) {
			break
		}

		var x GetVPSNameTasksId

		if err := c.Get("/vps/"+v+"/tasks/"+strconv.Itoa(i), &x); err != nil {
			return err
		}
		if _, ok := done[x.State]; ok {
			return nil
		}

		fmt.Printf("%d%%\n", x.Progress)
	}

	return fmt.Errorf("Rebuild pooling timeout")
}

func rebuildPoolResetKnownHosts(c *ovh.Client, v, i, kn string) error {
	x := PostInVPSNameRebuild{true, i, false, kn}
	var y PostOutVPSNameRebuild
	if err := c.Post("/vps/"+v+"/rebuild", &x, &y); err != nil {
		return err
	}
	if err := poolTask(c, v, y.Id); err != nil {
		return err
	}
	time.Sleep(waitVPSUp)
	return resetKnownHosts(c, v)
}

func main() {
	c, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	// XXX so far, this is useless, remove
	// ls-imgs is boilerplate free
	//	rmkeysCmd := flag.NewFlagSet("rm-keys", flag.ExitOnError)

	if len(os.Args) < 2 {
		help(1)
	}

	switch os.Args[1] {
	case "ls-apps":
		if err = lsApps(c); err != nil {
			log.Fatal(err)
		}
	case "rm-apps":
		for i := 2; i < len(os.Args); i++ {
			if err = rmApp(c, os.Args[i]); err != nil {
				log.Fatal(err)
			}
		}
	case "ls-vps":
		if err = lsVPS(c); err != nil {
			log.Fatal(err)
		}
	case "ls-keys":
		if err = lsKeys(c); err != nil {
			log.Fatal(err)
		}
	case "ls-imgs":
		if len(os.Args) <= 2 {
			help(1)
		}
		if err := lsImgs(c, os.Args[2]); err != nil {
			log.Fatal(err)
		}
	case "ls-img":
		if len(os.Args) <= 3 {
			help(1)
		}
		id, name, err := getMatchingImg(c, os.Args[2], os.Args[3])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\t%s\n", name, id)
	case "rebuild":
		if len(os.Args) <= 3 {
			help(1)
		}
		v := os.Args[2]
		i := os.Args[3]
		if !isImgId(i) {
			var err error
			var in string
			i, in, err = getMatchingImg(c, v, i)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Installing %s (%s) to %s\n", in, i, v)
		}
		kn := ovhKeyName
		if len(os.Args) > 4 {
			kn = os.Args[4]
		}
		if err := rebuildPoolResetKnownHosts(c, v, i, kn); err != nil {
			log.Fatal(err)
		}
	// shortcut
	case "rebuild-debian":
		if len(os.Args) <= 2 {
			help(1)
		}
		v := os.Args[2]
		i, in, err := getMatchingImg(c, v, "Debian")
		if err != nil {
			log.Fatal(err)
		}
		kn := ovhKeyName
		if len(os.Args) > 3 {
			kn = os.Args[3]
		}
		log.Printf("Installing %s (%s) to %s; key=%s\n", in, i, v, kn)
		if err := rebuildPoolResetKnownHosts(c, v, i, kn); err != nil {
			log.Fatal(err)
		}
	case "rm-keys":
		for i := 2; i < len(os.Args); i++ {
			if err = rmKey(c, os.Args[i]); err != nil {
				log.Fatal(err)
			}
		}
	case "add-key":
		kn := ovhKeyName
		var k string
		var err error
		if len(os.Args) <= 2 {
			if k, err = readSSHKey(); err != nil {
				log.Fatal(err)
			}
		}
		if len(os.Args) >= 3 {
			kn = os.Args[2]
		}
		if len(os.Args) >= 4 {
			s, err := os.ReadFile(os.Args[3])
			if err != nil && !os.IsNotExist(err) {
				log.Fatal(err)
			} else if err == nil {
				k = strings.TrimSuffix(string(s), "\n")
			} else {
				k = os.Args[3]
			}
		}
		fmt.Println(kn, k)
		if err = addKey(c, kn, k); err != nil {
			log.Fatal(err)
		}
	case "get-console":
		if len(os.Args) < 3 {
			help(1)
		}
		if err = getConsole(c, os.Args[2]); err != nil {
			log.Fatal(err)
		}
	case "ls-ips":
		if len(os.Args) < 3 {
			help(1)
		}
		if err = lsIPs(c, os.Args[2]); err != nil {
			log.Fatal(err)
		}
	case "help":
		help(0)
	default:
		help(1)
	}
}
