## sander


吉尔·桑达 (JIL sander) 由于节俭的美学和简洁的线条而闻名。
极简主义一向不愁其追随者，但是很少有设计师能够像吉尔·桑达 (JIL SANDER) 那样将其作为一种艺术而细细研究。轻如羽毛的上衣以及轻便夹克而闻名遐迩。

基于[Go语言中文网 - Golang中文社区](https://studygolang.com "Go语言中文网 - Golang中文社区") 源码,并保留其版权！！


### 主要变更   

> 1.环境变化,项目默认基于开发着GOPATH环境      
> 2.vendor依赖库,默认自带,不需要重新下载    
> 3.项目目录发生变化，统一以sander为项目根目录            


### 编译系统  

```         
cd $GOPATH/src/

git clone https://github.com/studygolang/sander.git

cd ./sander

make 

```      
之后会在./bin目录下查看二进制文件，如果想要改变目录可自行到cmd 目录下编译，或者是修改Makefile文件.

* docker image 可执行文件和网站所需要的静态文件     

#### docker image

```        
make docker     
```    
#### docker-compose

```           
docker-compose up 
```   
### docker mariadb 

首次运行 mariadb时,需设置密码   

```   
mkdir -p /data/mysql/data  
docker run --name mysql -p 3306:3306 -v /data/mysql/data:/var/lib/mysql -e MYSQL_ROOT_PASSWORD='root' -d mysql    
```           
* chown: changing ownership of '/var/lib/mysql/': Permission denied
* 请在docker run 后边加  --privileged=true  参数

在浏览器中输入：http://127.0.0.1:8088

接下来你会看到图形化安装界面，一步步照做吧。

* 如果之后有出现页面空白，请查看 error.log 是否有错误     
* fork + PR。如果有修改 js 和 css，请执行 gulp （需要先安装 gulp）
* 编译参数-ldflags "-w -s", 详情请转 https://golang.org/cmd/link/ 


### 删除所以已经停止的容器
```   
docker rm $(docker ps -a -q)  
```   

[代码质量查看](https://goreportcard.com/report/github.com/studygolang/sander)  
