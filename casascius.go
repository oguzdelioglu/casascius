package main

import (
	// "crypto/sha256"

	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DisgoOrg/disgohook"
	"github.com/DisgoOrg/disgohook/api"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/base58"
	"github.com/dustin/go-humanize"
	"github.com/gosuri/uilive"
	"github.com/hashicorp/go-memdb"
	"github.com/minio/sha256-simd"
	"golang.org/x/crypto/ripemd160"

	boom "github.com/bits-and-blooms/bloom/v3"
)

var walletList_opt *string = flag.String("w", "wallets.txt", "Wallet File => wallets.txt")
var phraseCount_opt *int = flag.Int("pc", 22, "Phrase Length => 22")

// var output_opt *string = flag.String("o", "falsepositive.txt", "Bloom Filter False Positive => falsepositive.txt")
var discord_opt *bool = flag.Bool("dc", false, "If Notify With Discord => false")
var webhook_id *string = flag.String("webhook_id", "", "Discord Webhook ID => webhook_id")
var webhook_token *string = flag.String("webhook_token", "", "Discord Webhook Token => webhook_token")
var thread_opt *int = flag.Int("t", 1, "Thread Count => 1")
var verbose_opt *bool = flag.Bool("v", false, "Log Console => false")
var bsgs *bool = flag.Bool("bsgs", true, "BSGS => false")
var rr *bool = flag.Bool("rr", true, "Random Range BSGS => true")
var step *uint64 = flag.Uint64("step", 100000000, "BSGS STEP COUNT => 100000000")

var addressList []string
var letters2 [57]string = [57]string{"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "M", "N", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "2", "3", "4", "5", "6", "7", "8", "9"}
var letters1 [58]string = [58]string{"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "M", "N", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
var eliptic *btcec.KoblitzCurve = btcec.S256()

type Wallet struct {
	addressUncompressed string
	addressRIPEM160     string
	passphrase          string
}

type DBAddress struct {
	// addressByte []byte
	address string
}

var botStartElapsed time.Time

//var start time.Time
var webhook api.WebhookClient
var writer *uilive.Writer
var total int64 = 0
var totalFound int64 = 0
var totalBalancedAddress uint64 = 0

var sbf *boom.BloomFilter
var it memdb.ResultIterator
var txn *memdb.Txn

func init() {
	botStartElapsed = time.Now()
}

func main() {

	hostname, _ := os.Hostname()
	runtime.GOMAXPROCS(runtime.NumCPU()) //Max Core
	rand.Seed(time.Now().UnixNano())     // Random komutunun ayn?? de??eri vermemesi i??in

	flag.Parse()

	bytesRead, _ := ioutil.ReadFile(*walletList_opt)
	file_content := string(bytesRead)
	addressList = strings.Split(strings.Replace(file_content, "\r\n", "\n", -1), "\n")
	addressCount := len(addressList)

	fmt.Println("_____Parameter Settings_____")
	fmt.Println("Address:", *walletList_opt)
	fmt.Println("Phrase Count:", *phraseCount_opt)
	// fmt.Println("Output:", *output_opt)
	fmt.Println("Thread:", *thread_opt)
	fmt.Println("Log Output:", *verbose_opt)
	fmt.Println("BSGS:", *bsgs)
	fmt.Println("Random Range:", *rr)
	fmt.Println("BSGS Step:", *step)
	fmt.Println("_____Info_____")

	fmt.Println("Total Address:", uint(addressCount))

	rand.Seed(time.Now().UnixNano()) // Random komutunun ayn?? de??eri vermemesi i??in

	sbf = boom.NewWithEstimates(uint(addressCount), 0.000001) //0.00000000000000000001  0.0000001

	for index, address := range addressList {
		addressList[index] = AddressToRIPEM160(address)
		//addressList[index] = address
		// test, _ := hex.DecodeString(AddressToRIPEM160(address))
		// fmt.Printf("%v %v %v\n", address, test, []byte(address))
	}
	//os.Exit(0)
	sort.Strings(addressList)

	for _, address := range addressList {
		//test := TestAddressToRIPEM160(address)
		//test, _ := hex.DecodeString(AddressToRIPEM160(address))

		// if *verbose_opt {
		// 	fmt.Printf("%v %v\n", address, []byte(address))
		// }

		sbf.Add([]byte(address))
	}

	//os.Exit(0)
	// // Create the DB schema
	// schema := &memdb.DBSchema{
	// 	"addresslist": &memdb.TableSchema{
	// 		Name: "address",
	// 		Indexes: map[string]*memdb.IndexSchema{
	// 			"id": &memdb.IndexSchema{
	// 				Name:    "id",
	// 				Unique:  true,
	// 				Indexer: &memdb.StringFieldIndex{Field: "address"},
	// 			},
	// 		},
	// 	},
	// }
	schema := &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"addressList": {
				Name: "addressList",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "address"},
					},
					// "addressByte": {
					// 	Name:    "addressByte",
					// 	Unique:  false,
					// 	Indexer: &memdb.StringFieldIndex{Field: "addressByte"},
					// },
				},
			},
		},
	}

	// Create a new data base
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		panic(err)
	}
	// Create a write transaction
	txn = db.Txn(true)

	for _, address := range addressList {
		if err := txn.Insert("addressList", DBAddress{address: address}); err != nil {
			panic(err)
		}
	}
	// Commit the transaction
	txn.Commit()
	// Create read-only transaction
	txn = db.Txn(false)
	defer txn.Abort()

	// List all the people
	it, err = txn.Get("addressList", "id")
	if err != nil {
		panic(err)
	}
	//fmt.Println("RIPEM 160 List")
	//for obj := it.Next(); obj != nil; obj = it.Next() {
	//p := obj.(*DBAddress)
	//fmt.Printf("%v %v\n", p.address, p.addressByte)
	//fmt.Printf("mystr:\t %v \n", []byte(p.ripem160Byte))
	//}

	// os.Exit(0)

	fmt.Println("Wallets Loaded")

	if *discord_opt && webhook_id != nil && webhook_token != nil {
		webhook, _ = disgohook.NewWebhookClientByToken(nil, nil, fmt.Sprintf("%s/%s", *webhook_id, *webhook_token))
	}

	if *discord_opt && webhook_id != nil && webhook_token != nil {
		webhook.SendContent("Hello I'm " + hostname)
	}

	// var wg sync.WaitGroup
	// for i := 1; i <= *thread_opt; i++ {
	// 	wg.Add(1)
	// 	go Brute(i, &wg)
	// }
	// wg.Wait()

	// if CheckWallet(sqliteDatabase, "1JiFuXz6PgbDbxwg7ptFxem7iAeyJwR9vx") { //Elapsed Time 0.0010004
	// 	fmt.Println("Buldum")
	// } else {
	// 	fmt.Println("Bulamad??m")
	// }
	//os.Exit(0)

	var wg sync.WaitGroup

	/*
	 * Tell the 'wg' WaitGroup how many threads/goroutines
	 *  that are about to run concurrently.
	 */
	wg.Add(*thread_opt)

	fmt.Println("Threads Starting")
	for i := 0; i < *thread_opt; i++ {

		/*
		 * Spawn a thread for each iteration in the loop.
		 * Pass 'i' into goroutine's function
		 * in order to make sure each goroutine use a different value for 'i'
		 */
		go func(i int) {
			// At the end of the goroutine, tell the WaitGroup that another thread has completed
			time.Sleep(1 * time.Second)
			defer wg.Done()
			if *bsgs {
				BruteBSGS(i, &wg)
			} else {
				Brute(i, &wg)
			}
			fmt.Printf("i: %v\n", i)
		}(i)
	}
	wg.Wait()
	fmt.Println("Threads Started")

	if !*verbose_opt {
		go Counter() //Stat
	}
	//Close Function
	sig := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sig
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
	//Close Function
}

func Counter() {
	writer = uilive.New()
	writer.Start()
	time.Sleep(time.Millisecond * 1000) //1 Second
	for {
		avgSpeed := avgSpeedCalc()
		fmt.Fprintf(writer, "Total Wallet = %v\nThread Count = %v\nElapsed Time = %v\nGenerated Wallet = %v\nGenerate Speed Avg(s) = %v\nChecking = %d\nTotal Balanced Address = %d\nFor Close ctrl+c\n", len(addressList), *thread_opt, time.Since(botStartElapsed).String(), humanize.Comma(total), humanize.Comma(avgSpeed), totalFound, totalBalancedAddress)
		time.Sleep(time.Millisecond * 1000) //1 Second
	}
	//writer.Stop() // flush and stop rendering
}

func avgSpeedCalc() int64 {
	return total / int64(time.Since(botStartElapsed).Seconds())
}

func BruteBSGS(id int, wg *sync.WaitGroup) {
	defer wg.Done()
	hasher := sha256.New()
	var chars string = generateMiniKey(*phraseCount_opt)
	// var chars string = "S58cqZM2KsWxCeBXZj0000"
	var randombsgsCount uint64 = 0
	fmt.Printf("Target=>%v Thread=>%02d\n", chars, id)
	for { ////Elapsed Time 0.0010003
		total++
		randombsgsCount++
		if randombsgsCount > *step {
			chars = generateMiniKey(*phraseCount_opt)
			if *verbose_opt {
				//fmt.Printf("New Target=>%v ", chars)
				fmt.Printf("New Target=>%v Bingo=>%d Total Address=>%v Thread=>%02d Threads=>%v Generated=>%v FP=>%d AvgSpeed(s)=>%v Elapsed(s)=>%v\n", chars, totalBalancedAddress, len(addressList), id, *thread_opt, humanize.Comma(total), totalFound, humanize.Comma(avgSpeedCalc()), time.Since(botStartElapsed).String())
			}
			randombsgsCount = 0
		}
		var randomPhrase string = CheckMinikey(chars)
		//var randomPhrase = "SymcR374ukWS48XCfENrDPGaYagFpG"
		//fmt.Println(chars)
		var randomWallet string = GenerateAddress(randomPhrase, hasher) //Elapsed Time 0.0010002
		//For test

		// if *verbose_opt {
		// 	fmt.Println(randomPhrase) //, AddressToRIPEM160(uaddr.EncodeAddress())
		// 	//SaveWallet(randomPhrase, "test.txt", hasher)
		// 	//fmt.Println(randomPhrase, " ", randomWallet) //, AddressToRIPEM160(uaddr.EncodeAddress())
		// }
		// SaveWallet(randomWallet, "balance_wallets.txt", hasher) //Test
		//fmt.Printf("%v\n", randomWallet)
		//SaveWallet(randomWallet, "test.txt")
		var ripem160Address string = AddressToRIPEM160(randomWallet)
		if sbf.Test([]byte(ripem160Address)) {
			if CheckWallet(ripem160Address) { //GeneratorFull(randomPhrase, hasher).addressRIPEM160
				fmt.Println("BINGO: " + randomPhrase)
				SaveWallet(randomPhrase, "balance_wallets.txt", hasher)
				totalBalancedAddress++
				if *discord_opt && webhook_id != nil && webhook_token != nil {
					webhook.SendContent(randomPhrase)
				}
				os.Exit(0)
			}
			// else {
			// 	SaveWallet(randomWallet, *output_opt, hasher)
			// }
			totalFound++
		}
		chars = increament_minikey(randomPhrase)
		//os.Exit(0)
	}
}

func Brute(id int, wg *sync.WaitGroup) {
	defer wg.Done()
	hasher := sha256.New()
	for { ////Elapsed Time 0.0010003
		total++
		// fmt.Println(id)
		var randomPhrase = RandomPhrase(*phraseCount_opt, hasher) //Elapsed Time 0.000999
		//var randomPhrase = "SymcR374ukWS48XCfENrDPGaYagFpG" //For test

		randomWallet := GenerateAddress(randomPhrase, hasher) //Elapsed Time 0.0010002

		if *verbose_opt {
			fmt.Println(randomPhrase, " ", randomWallet) //, AddressToRIPEM160(uaddr.EncodeAddress())
		}
		// SaveWallet(randomWallet, "balance_wallets.txt", hasher) //Test
		//fmt.Printf("%v\n", randomWallet)
		//SaveWallet(randomWallet, "test.txt")
		if sbf.Test([]byte(AddressToRIPEM160(randomWallet))) {
			if CheckWallet(GeneratorFull(randomPhrase, hasher).addressRIPEM160) {
				fmt.Println("BINGO: " + randomPhrase)
				SaveWallet(randomPhrase, "balance_wallets.txt", hasher)
				totalBalancedAddress++
				if *discord_opt && webhook_id != nil && webhook_token != nil {
					webhook.SendContent(randomPhrase)
				}
				os.Exit(0)
			}
			// else {
			// 	SaveWallet(randomWallet, *output_opt, hasher)
			// }
			totalFound++
		}
	}
}

func SHA256(hasher hash.Hash, input []byte) (hash []byte) {
	hasher.Reset()
	hasher.Write(input)
	hash = hasher.Sum(nil)
	return hash
}

func RandomPhrase(length int, hasher hash.Hash) string {
	var charstest string = generateMiniKey(length)
	return CheckMinikey(charstest)
}

func CheckMinikey(charstest string) string {
	i := 0
	for sha256.Sum256([]byte(charstest + "?"))[0] != '\x00' {
		charstest = increament_minikey(charstest)
		i++
		//fmt.Printf("Increament %v\n", charstest)
	}
	fmt.Printf("Tries %v Times\n", i)
	return charstest[:*phraseCount_opt]
}

func increament_minikey(charstest string) string {
	for i := len(charstest) - 1; i >= 0; i-- { //for i := len(charstest) - 2; i >= 0; i-- {
		c := rune(charstest[i])
		if c == '9' {
			charstest = replaceAtIndex(charstest, 'A', i)
			break
		} else if c == 'H' {
			charstest = replaceAtIndex(charstest, 'J', i)
			break
		} else if c == 'N' {
			charstest = replaceAtIndex(charstest, 'P', i)
			break
		} else if c == 'Z' {
			charstest = replaceAtIndex(charstest, 'a', i)
			break
		} else if c == 'k' {
			charstest = replaceAtIndex(charstest, 'm', i)
			break
		} else if c == 'z' {
			charstest = replaceAtIndex(charstest, '2', i)
		} else {
			c++
			charstest = replaceAtIndex(charstest, c, i)
			break
		}
	}
	return charstest
}

func replaceAtIndex(in string, r rune, i int) string {
	out := []rune(in)
	out[i] = r
	return string(out)
}

func generateMiniKey(length int) string {
	miniKey := "S"
	for i := 1; i < length; i++ {
		miniKey += letters1[rand.Intn(len(letters1))]
	}
	// charstest := miniKey + "?"
	//return miniKey, charstest
	return miniKey
	// return charstest
}

// func RandomPhrase(length int, hasher hash.Hash) string {
// 	var charstest string = generateMiniKey(length)
// 	//var charstest string = "SG64GZqySYwBm9KxE3wJ29?"
// 	// var miniKey, charstest = generateMiniKey(length)
// 	//fmt.Println(miniKey, sha[0], charstest)
// 	//os.Exit(0)

// 	// for SHA256(hasher, []byte(charstest))[0] != '\x00' {
// 	for sha256.Sum256([]byte(charstest))[0] != '\x00' {
// 		// As long as key doesn't pass typo check, increment it.
// 		// fmt.Println(charstest)
// 		for i := len(charstest) - 2; i >= 0; i-- {
// 			c := rune(charstest[i])
// 			if c == '9' {
// 				// miniKey = replaceAtIndex(miniKey, 'A', i)
// 				charstest = replaceAtIndex(charstest, 'A', i)
// 				break
// 			} else if c == 'H' {
// 				// miniKey = replaceAtIndex(miniKey, 'J', i)
// 				charstest = replaceAtIndex(charstest, 'J', i)
// 				break
// 			} else if c == 'N' {
// 				// miniKey = replaceAtIndex(miniKey, 'P', i)
// 				charstest = replaceAtIndex(charstest, 'P', i)
// 				break
// 			} else if c == 'Z' {
// 				// miniKey = replaceAtIndex(miniKey, 'a', i)
// 				charstest = replaceAtIndex(charstest, 'a', i)
// 				break
// 			} else if c == 'k' {
// 				// miniKey = replaceAtIndex(miniKey, 'm', i)
// 				charstest = replaceAtIndex(charstest, 'm', i)
// 				break
// 			} else if c == 'z' {
// 				// miniKey = replaceAtIndex(miniKey, '2', i)
// 				charstest = replaceAtIndex(charstest, '2', i)
// 				// No break - let loop increment prior character.
// 			} else {
// 				c++
// 				// miniKey = replaceAtIndex(miniKey, c, i)
// 				charstest = replaceAtIndex(charstest, c, i)
// 				break
// 				// fmt.Println(c)
// 			}
// 		}
// 	}
// 	// fmt.Println(charstest[:length], " End")
// 	//os.Exit(0)

// 	// if hashCheck[0] != '\x00' {
// 	// 	//hashCheck = hasher.Sum(sha)
// 	// 	goto check
// 	// }
// 	//fmt.Println(miniKey)
// 	// f, err := os.OpenFile("test.txt",
// 	// 	os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	// if err != nil {
// 	// 	log.Println(err)
// 	// }
// 	// defer f.Close()
// 	// if _, err := f.WriteString(miniKey + "\n"); err != nil {
// 	// 	log.Println(err)
// 	// }
// 	//fmt.Println(sha256.Sum256([]byte(charstest)))
// 	return charstest[:length]
// }

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

func GenerateAddress(passphrase string, hasher hash.Hash) string {

	var pk []byte = SHA256(hasher, []byte(passphrase))
	x, y := eliptic.ScalarBaseMult(pk)
	//ecdsa
	var publicKey = btcec.PublicKey{
		Curve: eliptic,
		X:     x,
		Y:     y,
	}

	hash160, _ := btcutil.NewAddressPubKey(publicKey.SerializeUncompressed(), &chaincfg.MainNetParams)
	// _, public := btcec.PrivKeyFromBytes(eliptic, SHA256(hasher, []byte(passphrase)))
	// hash160, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
	// return []byte(hash160.EncodeAddress())
	//fmt.Printf("%v %v\n", hash160.EncodeAddress(), []byte(hash160.EncodeAddress()))
	return hash160.EncodeAddress()
}

// func GeneratorRIPEMD160(passphrase string, hasher hash.Hash) []byte {
// 	//_, public := btcec.PrivKeyFromBytes(btcec.S256(), SHA256(hasher, []byte(passphrase)))
// 	_, public := btcec.PrivKeyFromBytes(eliptic, SHA256(hasher, []byte(passphrase)))
// 	//hash160 := btcutil.Hash160(public.SerializeUncompressed())
// 	// fmt.Printf("%v\n", public.SerializeUncompressed())
// 	//hash160 := btcutil.Hash160(public.SerializeUncompressed())
// 	hash160, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
// 	//fmt.Printf("%v %v\n", hash160.EncodeAddress(), []byte(hash160.EncodeAddress()))
// 	return []byte(hash160.EncodeAddress())
// }

func GeneratorFull(passphrase string, hasher hash.Hash) Wallet {
	// defer timeTrack(time.Now(), "GeneratorFull")
	// hasher := sha256.New() // SHA256
	// _, public := btcec.PrivKeyFromBytes(btcec.S256(), SHA256(hasher, []byte(passphrase)))
	_, public := btcec.PrivKeyFromBytes(btcec.S256(), SHA256(hasher, []byte(passphrase)))
	// Get compressed and uncompressed addresses
	// caddr, _ := btcutil.NewAddressPubKey(public.SerializeCompressed(), &chaincfg.MainNetParams)
	uaddr, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
	uadrEncoded := uaddr.EncodeAddress()
	if *verbose_opt {
		fmt.Println(passphrase, uadrEncoded) //, AddressToRIPEM160(uaddr.EncodeAddress())
	}
	return Wallet{addressRIPEM160: AddressToRIPEM160(uadrEncoded), addressUncompressed: uadrEncoded, passphrase: passphrase} // Send line to output channel
	// return Wallet{addressUncompressed: uaddr.EncodeAddress(), addressCompressed: caddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
}

// func GeneratorFull(passphrase string, hasher hash.Hash) Wallet {
// 	// defer timeTrack(time.Now(), "GeneratorFull")
// 	// hasher := sha256.New() // SHA256
// 	// _, public := btcec.PrivKeyFromBytes(btcec.S256(), SHA256(hasher, []byte(passphrase)))
// 	_, public := btcec.PrivKeyFromBytes(btcec.S256(), SHA256(hasher, []byte(passphrase)))
// 	// Get compressed and uncompressed addresses
// 	// caddr, _ := btcutil.NewAddressPubKey(public.SerializeCompressed(), &chaincfg.MainNetParams)
// 	uaddr, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
// 	uadrEncoded := uaddr.EncodeAddress()
// 	if *verbose_opt {
// 		fmt.Println(passphrase, uadrEncoded) //, AddressToRIPEM160(uaddr.EncodeAddress())
// 	}
// 	return Wallet{addressRIPEM160: AddressToRIPEM160(uadrEncoded), addressUncompressed: uadrEncoded, passphrase: passphrase} // Send line to output channel
// 	// return Wallet{addressUncompressed: uaddr.EncodeAddress(), addressCompressed: caddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
// }

// func GeneratorFull(passphrase string) Wallet {
// 	// defer timeTrack(time.Now(), "GeneratorFull")
// 	hasher := sha256.New() // SHA256
// 	sha := SHA256(hasher, []byte(passphrase))
// 	_, public := btcec.PrivKeyFromBytes(btcec.S256(), sha)

// 	// Get compressed and uncompressed addresses
// 	// caddr, _ := btcutil.NewAddressPubKey(public.SerializeCompressed(), &chaincfg.MainNetParams)
// 	uaddr, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
// 	if *verbose_opt {
// 		fmt.Println(passphrase, uaddr.EncodeAddress()) //, AddressToRIPEM160(uaddr.EncodeAddress())
// 	}
// 	return Wallet{addressRIPEM160: AddressToRIPEM160(uaddr.EncodeAddress()), addressUncompressed: uaddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
// 	// return Wallet{addressUncompressed: uaddr.EncodeAddress(), addressCompressed: caddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
// }

// SHA256 Hasher function

func SaveWallet(passphrase string, path string, hasher hash.Hash) {
	fullWallet := GeneratorFull(passphrase, hasher)
	f, err := os.OpenFile(path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fullWallet.passphrase + ":" + fullWallet.addressUncompressed + "\n"); err != nil {
		log.Println(err)
	}
}

func CheckWallet(hash string) bool {
	//fmt.Printf("%v", hash)
	it, err := txn.Get("addressList", "id", hash)
	// fmt.Printf("%v", hash)
	if err != nil {
		fmt.Printf("Error %v", err)
		return false
	}
	if it.Next() != nil {
		//fmt.Printf("Bingo %v", hash)
		return true
	}
	return false
}

func TestAddressToRIPEM160(address string) string {
	baseBytes := base58.Decode(address)
	end := len(baseBytes) - 4
	hash := baseBytes[0:end]
	return hex.EncodeToString(hash)[2:]
}

func AddressToRIPEM160(address string) string {
	hasher := ripemd160.New()
	hasher.Write([]byte(address))
	hashBytes := hasher.Sum(nil)
	hashString := fmt.Sprintf("%x", hashBytes)
	return hashString
}
