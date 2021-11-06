package main

import (
	"crypto/sha256"
	"database/sql"
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
	"github.com/gosuri/uilive"
	"github.com/jbenet/go-base58"

	boom "github.com/bits-and-blooms/bloom/v3"

	_ "github.com/mattn/go-sqlite3"
)

var walletList_opt *string = flag.String("w", "wallets.txt", "Wallet File => wallets.txt")
var walletInsert_opt *bool = flag.Bool("wi", false, "Wallet Insert => (Need Use For First Create)")
var phraseCount_opt *int = flag.Int("pc", 22, "Phrase Length => 22")
var output_opt *string = flag.String("o", "falsepositive.txt", "Bloom Filter False Positive => falsepositive.txt")
var discord_opt *bool = flag.Bool("dc", false, "If Notify With Discord => false")
var webhook_id *string = flag.String("webhook_id", "", "Discord Webhook ID => webhook_id")
var webhook_token *string = flag.String("webhook_token", "", "Discord Webhook Token => webhook_token")
var thread_opt *int = flag.Int("t", 1, "Thread Count => 1")
var verbose_opt *bool = flag.Bool("v", false, "Log Console => false")

var addressList []string
var letters [57]string = [57]string{"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "M", "N", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "2", "3", "4", "5", "6", "7", "8", "9"}

type Wallet struct {
	addressUncompressed string
	addressRIPEM160     string
	passphrase          string
}

var botStartElapsed time.Time

//var start time.Time
var webhook api.WebhookClient
var writer *uilive.Writer
var total uint64 = 0
var totalFound uint64 = 0
var totalBalancedAddress uint64 = 0
var BalanceAPI string = "https://sochain.com/api/v2/get_address_balance/bitcoin/" //API

var sqliteDatabase *sql.DB
var sbf *boom.BloomFilter

func main() {

	hostname, _ := os.Hostname()
	runtime.GOMAXPROCS(runtime.NumCPU()) //Max Core
	rand.Seed(time.Now().UnixNano())     // Random komutunun aynı değeri vermemesi için

	flag.Parse()

	bytesRead, _ := ioutil.ReadFile(*walletList_opt)
	file_content := string(bytesRead)
	addressList = strings.Split(strings.Replace(file_content, "\r\n", "\n", -1), "\n")
	addressCount := len(addressList)

	fmt.Println("_____Parameter Settings_____")
	fmt.Println("Address:", *walletList_opt)
	fmt.Println("Address Insert:", *walletInsert_opt)
	fmt.Println("Phrase Count:", *phraseCount_opt)
	fmt.Println("Output:", *output_opt)
	fmt.Println("Thread:", *thread_opt)
	fmt.Println("Log Output:", *verbose_opt)
	fmt.Println("_____Info_____")

	fmt.Println("Total Address:", uint(addressCount))

	rand.Seed(time.Now().UnixNano()) // Random komutunun aynı değeri vermemesi için

	sbf = boom.NewWithEstimates(uint(addressCount), 0.000001) //0.00000000000000000001  0.0000001

	for index, address := range addressList {
		addressList[index] = AddressToRIPEM160(address)
	}
	sort.Strings(addressList)

	for _, address := range addressList {
		//fmt.Println(address)
		sbf.Add([]byte(address))
	}

	fmt.Println("Wallets Loaded")

	if *discord_opt && webhook_id != nil && webhook_token != nil {
		webhook, _ = disgohook.NewWebhookClientByToken(nil, nil, fmt.Sprintf("%s/%s", *webhook_id, *webhook_token))
	}

	if *discord_opt && webhook_id != nil && webhook_token != nil {
		webhook.SendContent("Hello I'm " + hostname)
	}

	if *walletInsert_opt {
		createDB()
	}
	sqliteDatabase, _ = sql.Open("sqlite3", "database.db") // Open the created SQLite File
	defer sqliteDatabase.Close()                           // Defer Closing the database
	if *walletInsert_opt {
		InsertWalletToDB()
	}
	go Counter() //Stat
	// var wg sync.WaitGroup
	// for i := 1; i <= *thread_opt; i++ {
	// 	wg.Add(1)
	// 	go Brute(i, &wg)
	// }
	// wg.Wait()

	// if CheckWallet(sqliteDatabase, "1JiFuXz6PgbDbxwg7ptFxem7iAeyJwR9vx") { //Elapsed Time 0.0010004
	// 	fmt.Println("Buldum")
	// } else {
	// 	fmt.Println("Bulamadım")
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
			defer wg.Done()
			Brute(i, &wg)
			fmt.Printf("i: %v\n", i)
		}(i)
	}
	wg.Wait()
	fmt.Println("Threads Started")

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

func init() {
	botStartElapsed = time.Now()
}

func Counter() {
	writer = uilive.New()
	writer.Start()
	time.Sleep(time.Millisecond * 1000) //1 Second
	for {
		avgSpeed := total / uint64(time.Since(botStartElapsed).Seconds())
		fmt.Fprintf(writer, "Total Wallet = %v\nThread Count = %v\nElapsed Time = %v\nGenerated Wallet = %d\nGenerate Speed Avg(s) = %v\nChecking = %d\nTotal Balanced Address = %d\nFor Close ctrl+c\n", len(addressList), *thread_opt, time.Since(botStartElapsed).String(), total, avgSpeed, totalFound, totalBalancedAddress)
		time.Sleep(time.Millisecond * 1000) //1 Second
	}
	//writer.Stop() // flush and stop rendering
}

func Brute(id int, wg *sync.WaitGroup) {
	defer wg.Done()
	hasher := sha256.New()
	for { ////Elapsed Time 0.0010003
		total++
		// fmt.Println(id)
		var randomPhrase = RandomPhrase(*phraseCount_opt, hasher) //Elapsed Time 0.000999
		//var randomPhrase = "SymcR374ukWS48XCfENrDPGaYagFpG" //For test
		randomWallet := GeneratorFull(randomPhrase, hasher) //Elapsed Time 0.0010002
		// SaveWallet(randomWallet, "balance_wallets.txt", hasher) //Test
		//fmt.Println(randomWallet.base58BitcoinAddress)
		//SaveWallet(randomWallet, "test.txt")
		if sbf.Test([]byte(randomWallet.addressRIPEM160)) {
			if CheckWallet(sqliteDatabase, randomWallet.addressRIPEM160) {
				fmt.Println("BINGO: " + randomWallet.passphrase + " " + randomWallet.addressUncompressed)
				SaveWallet(randomWallet, "balance_wallets.txt", hasher)
				totalBalancedAddress++
				if *discord_opt && webhook_id != nil && webhook_token != nil {
					webhook.SendContent(randomWallet.passphrase + " " + randomWallet.addressUncompressed)
				}
				os.Exit(0)
			} else {
				SaveWallet(randomWallet, *output_opt, hasher)
			}
			totalFound++
		}
	}
}

func RandomPhrase(length int, hasher hash.Hash) string {
	var charstest string = generateMiniKey(length)
	// var charstest string = "SG64GZqySYwBm9KxE3wJ29?"
	// var miniKey, charstest = generateMiniKey(length)
	//fmt.Println(miniKey, sha[0], charstest)
	//os.Exit(0)
	for SHA256(hasher, []byte(charstest))[0] != '\x00' {
		// As long as key doesn't pass typo check, increment it.
		// fmt.Println(charstest)
		for i := len(charstest) - 2; i >= 0; i-- {
			c := rune(charstest[i])
			if c == '9' {
				// miniKey = replaceAtIndex(miniKey, 'A', i)
				charstest = replaceAtIndex(charstest, 'A', i)
				break
			} else if c == 'H' {
				// miniKey = replaceAtIndex(miniKey, 'J', i)
				charstest = replaceAtIndex(charstest, 'J', i)
				break
			} else if c == 'N' {
				// miniKey = replaceAtIndex(miniKey, 'P', i)
				charstest = replaceAtIndex(charstest, 'P', i)
				break
			} else if c == 'Z' {
				// miniKey = replaceAtIndex(miniKey, 'a', i)
				charstest = replaceAtIndex(charstest, 'a', i)
				break
			} else if c == 'k' {
				// miniKey = replaceAtIndex(miniKey, 'm', i)
				charstest = replaceAtIndex(charstest, 'm', i)
				break
			} else if c == 'z' {
				// miniKey = replaceAtIndex(miniKey, '2', i)
				charstest = replaceAtIndex(charstest, '2', i)
				// No break - let loop increment prior character.
			} else {
				c++
				// miniKey = replaceAtIndex(miniKey, c, i)
				charstest = replaceAtIndex(charstest, c, i)
				break
				// fmt.Println(c)
			}
		}
	}
	// fmt.Println(charstest[:length], " End")
	//os.Exit(0)

	// if hashCheck[0] != '\x00' {
	// 	//hashCheck = hasher.Sum(sha)
	// 	goto check
	// }
	//fmt.Println(miniKey)
	// f, err := os.OpenFile("test.txt",
	// 	os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	log.Println(err)
	// }
	// defer f.Close()
	// if _, err := f.WriteString(miniKey + "\n"); err != nil {
	// 	log.Println(err)
	// }

	return charstest[:length]
}
func replaceAtIndex(in string, r rune, i int) string {
	out := []rune(in)
	out[i] = r
	return string(out)
}

func generateMiniKey(length int) string {
	miniKey := "S"
	for i := 1; i < length; i++ {
		miniKey += letters[rand.Intn(len(letters))]
	}
	charstest := miniKey + "?"
	//return miniKey, charstest
	return charstest
}
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

func GeneratorFull(passphrase string, hasher hash.Hash) Wallet {
	// defer timeTrack(time.Now(), "GeneratorFull")
	// hasher := sha256.New() // SHA256
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
func SHA256(hasher hash.Hash, input []byte) (hash []byte) {
	hasher.Reset()
	hasher.Write(input)
	hash = hasher.Sum(nil)
	return hash
}

func SaveWallet(walletInfo Wallet, path string, hasher hash.Hash) {
	fullWallet := GeneratorFull(walletInfo.passphrase, hasher)
	f, err := os.OpenFile(path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fullWallet.addressUncompressed + ":" + fullWallet.passphrase + "\n"); err != nil {
		log.Println(err)
	}
}

func CheckWallet(db *sql.DB, hash string) bool {
	sqlStmt := `SELECT hash FROM address WHERE hash = ?`
	err := db.QueryRow(sqlStmt, hash).Scan(&hash)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Print(err)
		}
		return false
	}
	return true
}

func insertaddressBatch(db *sql.DB, hash string) {
	insertHash := `INSERT INTO address(hash) VALUES ` + hash
	statement, err := db.Prepare(insertHash)

	//println(insertHash)
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec()
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func createDB() {
	log.Println("Creating Database...")
	file, err := os.Create("database.db") // Create SQLite file
	if err != nil {
		log.Fatal(err.Error())
	}
	file.Close()
	log.Println("DB created")
}

func InsertWalletToDB() {
	createTable(sqliteDatabase) // Create Database Tables
	var tempCodes []string
	var currentIndex int = 0
	for _, address := range addressList {
		tempCodes = append(tempCodes, address) //append(tempCodes, address) //tempCodes = append(tempCodes, AddressToRIPEM160(address))
		if currentIndex == 5000 {
			code := `("` + strings.Join(tempCodes, `") , ("`) + `")`
			insertaddressBatch(sqliteDatabase, code)
			tempCodes = nil
			currentIndex = 0
		}
		currentIndex++
	}
	if len(tempCodes) <= 5000 {
		insertaddressBatch(sqliteDatabase, `("`+strings.Join(tempCodes, `") , ("`)+`")`)
		tempCodes = nil
	}
	fmt.Println("Wallets Inserted Databased.")
}

func createTable(db *sql.DB) {
	createHashTable := `CREATE TABLE "address" (
		"id"	INTEGER UNIQUE,
		"hash"	TEXT NOT NULL UNIQUE,
		PRIMARY KEY("id" AUTOINCREMENT)
	);` // SQL Statement for Create Table
	log.Println("Creating table...")
	statement, err := db.Prepare(createHashTable) // Prepare SQL Statement
	if err != nil {
		log.Fatal(err.Error())
	}
	statement.Exec() // Execute SQL Statements
	log.Println("Table created")
}

func AddressToRIPEM160(address string) string {
	baseBytes := base58.Decode(address)
	end := len(baseBytes) - 4
	hash := baseBytes[0:end]
	return hex.EncodeToString(hash)[2:]
}

func insertaddress(db *sql.DB, hash string) {
	insertHash := `INSERT INTO address(hash) VALUES (?)`
	statement, err := db.Prepare(insertHash)
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec(hash)
	if err != nil {
		log.Fatalln(err.Error())
	}
}
