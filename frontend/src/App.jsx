import React, { useState, useRef, useEffect } from 'react';
import { Box, Container } from '@mui/material';
import ChatBubble from './components/ChatBubble';
import ChatInput from './components/ChatInput';
import { GraphQLClient, gql } from 'graphql-request';

const App = () => {
 const [messages, setMessages] = useState([]);
 const chatContainerRef = useRef(null);
 const [papers, setPapers] = useState([]);
 const client = new GraphQLClient(
  'http://localhost:8686/graphql'
 );

 useEffect(() => {
   if (chatContainerRef.current) {
       chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
   }
 }, [messages]);

 const handleSendMessage = async (text) => {
    setMessages(prevMessages => [...prevMessages, { text: text, isUser: true, isUseful: true, fadeIn: false }]);

    const query = gql`
    query($messagesJSON: String!, $provider: String!, $papersJSON: String!) {
      queryAgent(messagesJSON: $messagesJSON, provider: $provider, papersJSON: $papersJSON) {
        item1
        item2
      }
    }
    `;

    try {
        const messagesJSON = JSON.stringify(messages);
        const papersJSON = JSON.stringify(papers);
        const provider = "gemini"; // or any other provider you are using

        const data = await client.request(query, { messagesJSON, provider, papersJSON });
        console.log(data);

        setMessages(prevMessages => [
          ...prevMessages,
          { text: data.queryAgent.item1, isUser: false, isUseful: false, fadeIn: true }
        ]);

        const newPapers = JSON.parse(data.queryAgent.item2);
        setPapers(newPapers);

        // Remove fadeIn after a short delay
        setTimeout(() => {
          setMessages(prevMessages => prevMessages.map((msg, index) => 
            index === prevMessages.length - 1 ? { ...msg, fadeIn: false } : msg
          ));
        }, 1000); // Adjust the delay as needed
    } catch (error) {
        console.error("Error in graphql request", error);
        setMessages(prevMessages => [
          ...prevMessages,
          { text: "Error processing your request", isUser: false, fadeIn: true }
        ]);

        // Remove fadeIn after a short delay
        setTimeout(() => {
          setMessages(prevMessages => prevMessages.map((msg, index) => 
            index === prevMessages.length - 1 ? { ...msg, fadeIn: false } : msg
          ));
        }, 1000); // Adjust the delay as needed
    }
 };

  return (
    <Container
      maxWidth="md"
      style={{
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
         }}
      >
      <Box
        flexGrow={1}
        overflow="auto"
        padding={2}
        ref={chatContainerRef}
        style={{ maxHeight: '70vh',}}
        >
        {messages.map((message, index) => (
          <ChatBubble
            key={index}
            text={message.text}
            isUser={message.isUser}
            fadeIn={message.fadeIn}
          />
        ))}
      </Box>
      <Box padding={2}>
         <ChatInput onSendMessage={handleSendMessage} />
      </Box>
    </Container>
  );
};

export default App;