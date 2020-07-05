print('start compare')
with open('serversend.txt', 'r') as file:
    serverSend = file.read()
    with open('clientread.txt', 'r') as file:
        clientRec = file.read()
        serverList = serverSend.splitlines()
        clientList = clientRec.splitlines()
        sendSet = set(serverList)
        clientSet = set(clientList)
        print(len(sendSet))
        print(len(clientSet))
        print(len(serverList))
        print(len(clientList))
        for item in clientSet:
            sendSet.discard(item)
        print(sendSet)