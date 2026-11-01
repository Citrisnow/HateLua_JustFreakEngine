#Hate Lua Just Freak Engine
做着玩的一个项目，可以理解是一个**lua中文版本**吧，当然覆盖的也不全，强绑定**Freak Engine**生态，未来会持续优化

本项目提供了一个**.hate**的编译器配置(CLT)，能够将符合特定语法的“中文 Lua 代码”，翻译成标准的英语Lua代码，并处理Freak Engine生态中的各种API映射

##特点
- **中文关键字支持**：支持将 Lua 关键字（如 `and`、`function`、`true`）替换为中文（如 `且`、`函数定义`、`真`）。
- **双向对象/方法映射**：支持 `对象:中文方法()`、`对象.中文方法()` 等多种调用形式，自动根据白名单解析为对应的英文 API。
- **智能变量编码**：遇到未映射的中文变量名，自动采用 `a, b, c ... aa, ab` 等稳定的短英文进行替换，并在首行自动插入注释映射，保证代码结构不冲突。
- **参数语义推断**：基于函数签名（如 `tag`、`var`、`fstr` 等）推断实参语义，精确控制哪些字符串或变量需要被翻译。
- **高度可定制的映射表**：所有翻译规则、对象、方法、白名单均通过 JSON 文件配置，方便扩展。

##配置文件
`hate` 编译器在运行时，会从当前工作目录**向上递归查找**名为 `build.hate` 的配置文件。该文件使用与 `haxe` 类似的键值对格式（用空格分隔）。
你可以通过命令 `hate init` 快速生成一个默认的配置文件。

##配置项说明

配置项 说明 示例
-cp 必填。源目录，指定需要扫描和转译的中文 Lua 代码所在文件夹。 -cp ./src
-out 必填。输出目录，指定转译后的标准 Lua 代码保存位置。 -out ./build
-all 是否编译源目录下的所有 .lua 文件。默认为 true。若为 false，需配合 -select。 -all true
-select 当 -all false 时，指定需要单独编译的某个 .lua 文件路径。 -select ./src/main.lua
-modify 是否使用当前工作区（根目录）下的 mapping/ 自定义映射表。默认为 false（使用编译器内置的映射表）。 -modify true
-reqimp 目标导入关键字。支持 import 或 require。默认为 require。 -reqimp require

## 命令行用法
hate 提供了几个基本的子命令来管理你的项目。

1. hate init [dir]

在指定目录（默认当前目录）下生成一个初始的 build.hate 配置文件。

· 例子：hate init 或 hate init ./my_project

2. hate compile [options]

执行编译转译操作。如果在当前目录找不到 build.hate，它会自动向父级目录寻找。
如果你在命令行里传入了参数，命令行的参数优先级高于配置文件中的参数。

· 可用选项：
  · --config <file>：强制指定配置文件路径（不再向上查找）。
  · -cp <dir>：覆盖配置文件的源目录。
  · -out <dir>：覆盖配置文件的输出目录。
  · -all <true|false>：覆盖是否全量编译。
  · -select <file>：覆盖单文件编译。
  · -modify <true|false>：覆盖是否使用工作区自定义映射表。
  · -reqimp <import|require>：覆盖目标导入关键字。
· 例子：
  · 使用配置文件直接编译：hate compile
  · 强制编译单个文件：hate compile -all false -select ./scripts/test.lua
  · 临时修改输出目录：hate compile -out ./dist

3. hate version / hate -v

显示当前编译器的版本信息（当前为 hate 1.0.0）。

4. hate help / hate -h

显示命令行帮助菜单。

## 安装
使用windows中的编辑账户环境变量 点击PATH 进行编辑 然后再点击新建 再把你的路径名写进去就可以在命令行使用了
比如**D:\relevant\bin** 然后你把你下载的包 在D盘中新建一个名字叫"relevant"的文件夹 再把你下载的东西解压到你刚刚新建的relevant文件夹中 就可以了
