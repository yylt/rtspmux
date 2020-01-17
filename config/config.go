package config

import (
	"time"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/spf13/pflag"
)

type containerformat int

const (
	HlsFmt containerformat =iota
	FlvFmt
	UnknownFmt
)

func validFormat(s string) (containerformat,bool){
	switch s {
	case "hls":
		return HlsFmt,true
	case "flv":
		return FlvFmt,true
	}
	return UnknownFmt,false
}

func (f *containerformat) String() string{
	switch *f {
	case HlsFmt:
		return "hls"
	case FlvFmt:
		return "flv"
	}
	return "unknown"
}

type SaveConfig struct {
	Interval time.Duration //每个文件的记录时长
	Max time.Duration
	Dir string
}


type Config struct {
	Froms []string
	Outformat containerformat
	Save *SaveConfig
	Addr string
	Certf string
	Keyf string
}

func init() {
	pflag.StringSlice("froms",[]string{},"upper streams, such as rtsp://192.168.1.1:554/camera")
	pflag.String("outformat","hls","out format,support hls,flv")
	pflag.String("save.interval","","save mp4 file interval")
	pflag.String("save.max","","save mp4 file max time")
	pflag.String("save.dir","","save mp4 file dir")
	pflag.String("addr",":1993","listen addr")
	pflag.String("cert","","cert file path")
	pflag.String("key","","key file path")
	pflag.String("conf","","config file,support json,yaml,toml")
}

func parse() {
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)
	printVersion()
}

func mustFromViper(c *Config) {
	if c==nil{
		panic("config is nil")
	}
	c.Froms=viper.GetStringSlice("froms")
	outf,ok:=validFormat(viper.GetString("outformat"))
	if !ok{
		panic(fmt.Sprintf("outformat not support: %s, only hls,flv",viper.GetString("outformat")))
	}
	c.Outformat=outf
	if s:=viper.GetString("save.interval");s!=""{
		du,err:=time.ParseDuration(s)
		if err!= nil{
			panic(err)
		}
		c.Save.Interval=du
	}

	if s:=viper.GetString("save.max");s!=""{
		du,err:=time.ParseDuration(s)
		if err!= nil{
			panic(err)
		}
		c.Save.Max=du
	}

	c.Save.Dir=viper.GetString("save.dir")
	c.Addr = viper.GetString("addr")
}

func mustConfigFromFile(fpath string) Config{
	f,err:=os.Open(fpath)
	if err!= nil{
		panic(err)
	}
	if strings.HasSuffix(fpath,"yaml"){
		viper.SetConfigType("yaml")
	}
	if strings.HasSuffix(fpath,"json"){
		viper.SetConfigType("json")
	}
	if strings.HasSuffix(fpath,"toml"){
		viper.SetConfigType("toml")
	}
	var c Config
	viper.ReadConfig(f)
	f.Close()
	mustFromViper(&c)
	return c
}

func ConfigRead() *Config{
	var (
		conf Config
	)
	conf.Save = new(SaveConfig)
	parse()
	mustFromViper(&conf)
	fpath := viper.GetString("conf")
	if fpath!= ""{
		conf = mustConfigFromFile(fpath)
	}
	return &conf
}

func (c *Config) Valid() error{
	if c.Save.Dir!="" {
		info,err:=os.Stat(c.Save.Dir)
		if err!= nil{
			return err
		}
		if info.IsDir()==false{
			return fmt.Errorf("%s is not directory",c.Save.Dir)
		}
	}
	if c.Save.Interval!=0{
		if c.Save.Interval > c.Save.Max{
			return fmt.Errorf("save interval bigger than max")
		}
	}
	return nil
}