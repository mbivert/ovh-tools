package main

// if need be, for more advanced logging, see:
//	https://github.com/golang/glog

import (
	"flag"
	"fmt"
	"github.com/ovh/go-ovh/ovh"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"
	"path/filepath"
	"strings"
)

// https://api.ovh.com/console/#/me/~GET
// TODO: incomplete
type GetMe struct {
	Email           string `json:"email"`
	Country         string `json:"country"`
	FirstName       string `json:"firstname"`
}

// https://api.ovh.com/console/#/me/api/application~GET
// TODO: ALPHA API
type GetMeApiApplication []int

// https://api.ovh.com/console/#/me/api/credential~GET
// TODO: ALPHA API
type GetMeApiCredential []int

// https://api.ovh.com/console/#/me/api/application/%7BapplicationId%7D~GET
//	Retrieve meta-data associated to an application ID
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
	Method         string `json:"method"`
	Path           string `json:"path"`
}
type GetMeApiCredentialId struct {
	// XXX type uncertain
	AllowedIPs     []string    `json:"allowedIPs"`
	// NOTE: 115/168 seem to correspond to the web console
	ApplicationId  int         `json:"applicationId"`
	Creation       time.Time   `json:"creation"`
	CredentialId   int         `json:"credentialId"`
	Expiration     time.Time   `json:"expiration"`
	LastUse        time.Time   `json:"lastUse"`
	OvhSupport     bool        `json:"ovhSupport"`
	Status         string      `json:"status"`
	Rules          []GetMeApiCredentialIdRule  `json:"rules"`
}

// https://api.ovh.com/console/#/me/api/credential/%7BcredentialId%7D~DELETE
// TODO: ALPHA API
type DeleteMeApiCredentialId struct {}

// https://api.ovh.com/console/#/vps~GET
type GetVps []string

// https://api.ovh.com/console/#/vps/%7BserviceName%7D~GET
type GetVpsIdModel struct {
	VCore    int           `json:"vcore"`
	Disk     int           `json:"disk"`
	Memory   int           `json:"memory"`
	// TODO: incomplete
}
type GetVpsId struct {
	State    string        `json:"state"`
	VCore    int           `json:"vcore"`
	Model    GetVpsIdModel `json:"model"`
	// TODO: incomplete
}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/ips~GET
type GetVpsIdIps []string

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/datacenter~GET
type GetVpsIdDatacenter struct {
	LongName   string        `json:"longName"`
	Name       string        `json:"name"`
	Country    string        `json:"country"`
}

// https://api.ovh.com/console/#/vps/%7BserviceName%7D/getConsoleUrl~POST
type PostInVpsIdGetConsole struct {}
type PostOutVpsIdGetConsole string

// https://api.ovh.com/console/#/me/api/application/%7BapplicationId%7D~DELETE
type DeleteMeApiApplicationId struct {}

// https://api.ovh.com/console/#/me/sshKey~GET
type GetMeSSHKey []string

// https://api.ovh.com/console/#/me/sshKey/%7BkeyName%7D~GET
type GetMeSSHKeyName struct {
	Key         string `json:"key"`
	KeyName     string `json:"keyName"`
	Default     bool   `json:"default"`
}

// https://api.ovh.com/console/#/me/sshKey/%7BkeyName%7D~DELETE
type DeleteMeSSHKeyName struct{}

// https://api.ovh.com/console/#/me/sshKey~POST
type PostInMeSSHKey struct {
	Key     string  `json:"key"`
	KeyName string  `json:"keyName"`
}
type PostOutMeSSHKey struct {}

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

var poolTimeout = 2 * time.Minute
var confFn = os.Getenv("HOME") +"/.ovh.conf"

// Used after a Ckrequest:  the CkRequest will register the new
// customer key for use in the client; hence, all (authenticated)
// requests will now fail until the credential has been validated.
func poolForValidated(c *ovh.Client) error {
	a := time.Now().Add(poolTimeout)
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
	fmt.Println("Waiting for credentials to be validated...");

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
		return nil, fmt.Errorf("Flushing expired credentials: nil", err)
	}

	return c, nil
}

func help(n int) {
	fmt.Println("TODO")
	os.Exit(n)
}

func foreachApp(c *ovh.Client, f func(*ovh.Client, *GetMeApiApplicationId) (error, bool)) error {
	var xs GetMeApiApplication
	var y GetMeApiApplicationId
	if err := c.Get("/me/api/application", &xs); err != nil {
		return err
	}
	for _, x := range xs {
		if err := c.Get("/me/api/application/"+strconv.Itoa(x), &y); err != nil {
			return err
		}
		err, stop := f(c, &y)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return nil
}

func lsapps(c *ovh.Client) error {
	return foreachApp(c, func(_ *ovh.Client, y *GetMeApiApplicationId) (error, bool) {
		fmt.Printf("%s %d %s %s\n", y.Name, y.ApplicationId, y.Status, y.Description)
		return nil, false
	})

}

// NOTE: we assume a to either be an integer (ie. an ID) or
// an app name. We could be smarter.
func rmapp(c *ovh.Client, a string) error {
	var z DeleteMeApiApplicationId
	id, err := strconv.Atoi(a)

	if err != nil {
		id = -1
		err := foreachApp(c, func(_ *ovh.Client, y *GetMeApiApplicationId) (error, bool) {
			if y.Name == a {
				id = y.ApplicationId
				return nil, true
			}
			return nil, false
		})
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

func lsvps(c *ovh.Client) error {
	var xs GetVps
	if err := c.Get("/vps", &xs); err != nil {
		return err
	}

	for _, x := range xs {
		var y GetVpsId
		var ips GetVpsIdIps
		var dc GetVpsIdDatacenter
		if err := c.Get("/vps/"+x, &y); err != nil {
			return err
		}
		if err := c.Get("/vps/"+x+"/ips", &ips); err != nil {
			return err
		}
		if err := c.Get("/vps/"+x+"/datacenter", &dc); err != nil {
			return err
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
	}
	return nil
}

func getconsole(c *ovh.Client, v string) error {
	var in PostInVpsIdGetConsole
	var out PostOutVpsIdGetConsole

	if err := c.Post("/vps/"+v+"/getConsoleUrl", &in, &out); err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func lsips(c *ovh.Client, v string) error {
	var ips GetVpsIdIps
	if err := c.Get("/vps/"+v+"/ips", &ips); err != nil {
		return err
	}
	for _, ip := range ips {
		fmt.Println(ip)
	}
	return nil
}

func lskeys(c *ovh.Client) error {
	var xs GetMeSSHKey
	var y GetMeSSHKeyName

	if err := c.Get("/me/sshKey", &xs); err != nil {
		return err
	}

	for _, x := range xs {
		if err := c.Get("/me/sshKey/"+x, &y); err != nil {
			return err
		}
		fmt.Printf("%s %s\n", y.KeyName, y.Key)
	}

	return nil
}

func rmkey(c *ovh.Client, n string) error {
	var x DeleteMeSSHKeyName
	return c.Delete("/me/sshKey/"+n, &x)
}

func addkey(c *ovh.Client, n, v string) error {
	x := PostInMeSSHKey{v, n}
	var y PostOutMeSSHKey
	return c.Post("/me/sshKey/", &x, &y)
}

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
var ovhKeyName = "ovh-do-key2"

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

func main() {
	c, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	// XXX so far, this is useless.
	lsvpsCmd := flag.NewFlagSet("ls-vps", flag.ExitOnError)
	lsappsCmd := flag.NewFlagSet("ls-apps", flag.ExitOnError)
	rmappsCmd := flag.NewFlagSet("rm-apps", flag.ExitOnError)
	getconsoleCmd := flag.NewFlagSet("get-console", flag.ExitOnError)
	lsipsCmd := flag.NewFlagSet("ls-ips", flag.ExitOnError)
	lskeysCmd := flag.NewFlagSet("ls-keys", flag.ExitOnError)
	rmkeysCmd := flag.NewFlagSet("rm-keys", flag.ExitOnError)
	addkeyCmd := flag.NewFlagSet("add-key", flag.ExitOnError)

	if len(os.Args) < 2 {
		help(1)
	}

	switch os.Args[1] {
	case "ls-apps":
		lsappsCmd.Parse(os.Args[2:])
		if err = lsapps(c); err != nil {
			log.Fatal(err)
		}
	case "rm-apps":
		rmappsCmd.Parse(os.Args[2:])
		for i := 0; i < rmappsCmd.NArg(); i++ {
			if err = rmapp(c, rmappsCmd.Arg(0)); err != nil {
				log.Fatal(err)
			}
		}
	case "ls-vps":
		lsvpsCmd.Parse(os.Args[2:])
		if err = lsvps(c); err != nil {
			log.Fatal(err)
		}
	case "ls-keys":
		lskeysCmd.Parse(os.Args[2:])
		if err = lskeys(c); err != nil {
			log.Fatal(err)
		}
	case "rm-keys":
		rmkeysCmd.Parse(os.Args[2:])
		for i := 0; i < rmkeysCmd.NArg(); i++ {
			if err = rmkey(c, rmkeysCmd.Arg(0)); err != nil {
				log.Fatal(err)
			}
		}
	case "add-key":
		addkeyCmd.Parse(os.Args[2:])
		name := ovhKeyName
		var key string
		var err error
		if addkeyCmd.NArg() <= 1 {
			if key, err = readSSHKey(); err != nil {
				log.Fatal(err)
			}
		}
		if addkeyCmd.NArg() >= 1 {
			name = addkeyCmd.Arg(0)
		}
		if addkeyCmd.NArg() >= 2 {
			s, err := os.ReadFile(addkeyCmd.Arg(1))
			if err != nil && !os.IsNotExist(err) {
				log.Fatal(err)
			} else if err == nil {
				key = strings.TrimSuffix(string(s), "\n")
			} else {
				key = addkeyCmd.Arg(1)
			}
		}
		fmt.Println(name, key)
		if err = addkey(c, name, key); err != nil {
			log.Fatal(err)
		}
	case "get-console":
		getconsoleCmd.Parse(os.Args[2:])
		if getconsoleCmd.NArg() == 0 {
			help(1)
		}
		if err = getconsole(c, getconsoleCmd.Arg(0)); err != nil {
			log.Fatal(err)
		}
	case "ls-ips":
		lsipsCmd.Parse(os.Args[2:])
		if lsipsCmd.NArg() == 0 {
			help(1)
		}
		if err = lsips(c, lsipsCmd.Arg(0)); err != nil {
			log.Fatal(err)
		}
	case "help":
		help(0)
	default:
		help(1)
	}
}
