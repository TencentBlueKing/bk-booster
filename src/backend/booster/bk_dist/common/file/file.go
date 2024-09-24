/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package file

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// Stat do the os.Stat and return an Info
func Stat(fp string) *Info {
	info, err := os.Stat(fp)
	return &Info{
		filePath: fp,
		info:     info,
		err:      err,
	}
}

// Lstat do the os.Lstat and return an Info
func Lstat(fp string) *Info {
	info, err := os.Lstat(fp)
	return &Info{
		filePath: fp,
		info:     info,
		err:      err,
	}
}

// get file info by enum dir
// TODO : not ok for linux, do not call in linux
func GetFileInfoByEnumDir(fp string) *Info {
	fis, err := ioutil.ReadDir(filepath.Dir(fp))
	if err != nil {
		return &Info{
			filePath: fp,
			info:     nil,
			err:      err,
		}
	} else {
		for _, fi := range fis {
			if strings.EqualFold(fi.Name(), filepath.Base(fp)) {
				newfp := fp
				if !strings.HasSuffix(fp, fi.Name()) {
					old := filepath.Base(fp)
					new := fi.Name()
					i := strings.LastIndex(fp, old)
					if i >= 0 {
						newfp = fp[:i] + new + fp[i+len(old):]
						blog.Infof("common file: from [%s] to [%s]", fp, newfp)
					}
				}

				return &Info{
					filePath: newfp,
					info:     fi,
					err:      err,
				}
			}
		}

		return &Info{
			filePath: fp,
			info:     nil,
			err:      fmt.Errorf("%s not existed", fp),
		}
	}
}

// Info describe the os.FileInfo and handle some actions
type Info struct {
	filePath   string
	LinkTarget string

	// info and err are return from os.Stat
	info os.FileInfo
	err  error
}

// Path return the file path
func (i *Info) Path() string {
	return i.filePath
}

// Basic return the origin os.FileInfo
func (i *Info) Basic() os.FileInfo {
	return i.info
}

// Error return the os.Stat/os.Lstat error
func (i *Info) Error() error {
	return i.err
}

// Exist check if the file is exist
func (i *Info) Exist() bool {
	return i.info != nil && (i.err == nil || os.IsExist(i.err))
}

// ModifyTime return the nano-second of this file's mod time
func (i *Info) ModifyTime() int64 {
	if i.info == nil {
		return 0
	}
	return i.info.ModTime().UnixNano()
}

// ModifyTime64 return the ModifyTime as int64
func (i *Info) ModifyTime64() int64 {
	return int64(i.ModifyTime())
}

// Mode32 return the file mode
func (i *Info) Mode32() uint32 {
	if i.info == nil {
		return 0
	}
	return uint32(i.info.Mode())
}

// Size return the file size
func (i *Info) Size() int64 {
	if i.info == nil {
		return 0
	}
	return i.info.Size()
}

// Batch return Exist, Size, ModifyTime64, Mode32 in one call.
func (i *Info) Batch() (bool, int64, int64, uint32) {
	return i.Exist(), i.Size(), i.ModifyTime64(), i.Mode32()
}

// Md5 return the md5 of this file
func (i *Info) Md5() (string, error) {
	if i.err != nil {
		return "", i.err
	}

	f, err := os.Open(i.filePath)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()
	}()

	md5hash := md5.New()
	if _, err := io.Copy(md5hash, f); err != nil {
		return "", err
	}

	md5string := fmt.Sprintf("%x", md5hash.Sum(nil))
	return md5string, nil
}

var (
	fileInfoCacheLock sync.RWMutex
	fileInfoCache     = map[string]*Info{}
)

func ResetFileInfoCache() {
	fileInfoCacheLock.Lock()
	defer fileInfoCacheLock.Unlock()

	fileInfoCache = map[string]*Info{}
}

// 支持并发read，但会有重复Stat操作，考虑并发和去重的平衡
func GetFileInfo(fs []string, mustexisted bool, notdir bool, statbysearchdir bool) ([]*Info, error) {
	// read
	fileInfoCacheLock.RLock()
	notfound := []string{}
	is := make([]*Info, 0, len(fs))
	for _, f := range fs {
		i, ok := fileInfoCache[f]
		if !ok {
			notfound = append(notfound, f)
			continue
		}

		if !i.Exist() {
			if mustexisted {
				// continue
				// TODO : return fail if not existed
				blog.Warnf("common util: depend file:%s not existed ", f)
				return nil, fmt.Errorf("%s not existed", f)
			} else {
				continue
			}
		}

		if notdir && i.Basic().IsDir() {
			continue
		}
		is = append(is, i)
	}
	fileInfoCacheLock.RUnlock()

	blog.Infof("common util: got %d file stat and %d not found", len(is), len(notfound))
	if len(notfound) == 0 {
		return is, nil
	}

	// query
	tempis := make(map[string]*Info, len(notfound))
	for _, notf := range notfound {
		tempf := notf
		try := 0
		maxtry := 10
		for {
			var i *Info
			if statbysearchdir {
				i = GetFileInfoByEnumDir(tempf)
			} else {
				i = Lstat(tempf)
			}
			tempis[tempf] = i
			try++

			if !i.Exist() {
				if mustexisted {
					// TODO : return fail if not existed
					// continue
					blog.Warnf("common util: depend file:%s not existed ", tempf)
					return nil, fmt.Errorf("%s not existed", tempf)
				} else {
					// continue
					break
				}
			}

			loopagain := false
			if i.Basic().Mode()&os.ModeSymlink != 0 {
				originFile, err := os.Readlink(tempf)
				if err == nil {
					if !filepath.IsAbs(originFile) {
						originFile, err = filepath.Abs(filepath.Join(filepath.Dir(tempf), originFile))
						if err == nil {
							i.LinkTarget = originFile
							blog.Infof("common util: symlink %s to %s", tempf, originFile)
						} else {
							blog.Infof("common util: symlink %s origin %s, got abs path error:%s",
								tempf, originFile, err)
						}
					} else {
						i.LinkTarget = originFile
						blog.Infof("common util: symlink %s to %s", tempf, originFile)
					}

					// 如果是链接，并且指向了其它文件，则需要将指向的文件也包含进来
					// 增加寻找次数限制，避免死循环
					if try < maxtry {
						loopagain = true
						tempf = originFile
					}
				} else {
					blog.Infof("common util: symlink %s Readlink error:%s", tempf, err)
				}
			}

			if notdir && i.Basic().IsDir() {
				continue
			}

			is = append(is, i)

			if !loopagain {
				break
			}
		}
	}

	// write
	go func(tempis *map[string]*Info) {
		fileInfoCacheLock.Lock()
		for f, i := range *tempis {
			fileInfoCache[f] = i
		}
		fileInfoCacheLock.Unlock()
	}(&tempis)

	return is, nil
}
