# AppChatty
Chat for DS Lab (SNE)

## Technology stack:
- Golang
- GTK3+ (gotk3)
- GORM / sqlite3

## Chat messages

Each packet has a structure defined below. Each packet should begin with `uint16` **data length** which is followed by `uint16` **operation code**. After this, actual **data** of the packets begins. We implemented following packets.

### List of operations:

#### 1: Message. Data: 
- SenderID `uint64` 
- UserID `uint64` (0 if not defined)
- GroupID `uint64` (0 if not defined)
- MessageLen `uint16`
- MessageContent `utf8`

#### 4: Register. Data:
- NameLen `byte`
- Name `utf8`
- PasswordLen `byte`
- Password `utf8`

#### 5: Authentication. Data:
- NameLen `byte`
- Name `utf8`
- PasswordLen `byte`
- Password `utf8`

#### 6: Get User ID by name. Data:
- nameLen `byte`
- name `utf8` 
Response 404 or 200 with data:
- UserID `uint64`

#### 7: Get name by User ID . Data:
- UserID `uint64`
Response 404 or 200 with data:
- nameLen `byte`
- name `utf8` 

#### 8: Get Chat History Exists. Data:
- UserID `uint64`
- GroupID `uint64`
- Offset `uint32`
- Count `uint32`

#### 10: Subscription Connection. Data:
- NameLen `byte`
- Name `utf8`
- PasswordLen `byte`
- Password `utf8`


### List of responses: 
- 200: OK. No data.
- 401: Unauthorized. No data.
- 404: Not found. No data. Used to notify that user doesn't exist.
- 406: Not Acceptable. No data. Used to notify that data is not valid.
- 423: Locked. No data. Used to notify a user that password is wrong.